---
name: setup
description: Configure the povr Pushover CLI for this machine — app token and user key resolution, the TOML config file used by daemons, and the Open Client session needed to receive messages. Use when povr reports a missing credential, when a povr command fails with "missing app_token credential" or "no session found", or when the user asks to set up, configure, or connect Pushover.
allowed-tools: Bash(povr validate:*), Bash(povr helper install), Bash(povr helper status), Bash(povr client login), Bash(povr client register:*), Bash(export PUSHOVER_NOTIFY_BACKEND=*), Bash(povr client listen:*), Bash(povr daemon install), Bash(povr daemon start), Bash(povr daemon stop), Bash(povr daemon restart), Bash(povr daemon status:*)
---

# Configure povr

Two independent halves. Sending needs credentials only. Receiving additionally needs an
Open Client session, and the macOS helper is the recommended notification backend for
the fullest feature set (custom icons, sounds, levels); without it povr falls back to
alerter, if installed, or osascript with reduced fidelity.
Set up only the half the task requires.

## Credentials

Roles: `app_token` (the Pushover application token), `user_key` (the recipient), and
`team_token` (only for `povr team ...`). Every command resolves the roles it needs in
this order:

1. The environment variable **named** by a `--*-env` flag, when one is passed:
   `--token-env` (app_token), `--user-env` (user_key), `--team-env` (team_token),
   `--key-env` (the command's primary role — `app_token` everywhere except
   `povr team ...`, where it is `team_token`). Role-specific flags win over
   `--key-env`. This path is fail-closed: if the named variable is unset or empty the
   command fails and nothing else is tried.
2. The role's default variable: `PUSHOVER_APP_TOKEN`, `PUSHOVER_USER_KEY`,
   `PUSHOVER_TEAM_TOKEN`.
3. `PUSHOVER_KEY`, but only for the command's primary role.
4. The TOML config file. Environment values always win over the file.

The `--*-env` flags carry a variable name, never a secret. Never put a token or key on
a command line.

```sh
export PUSHOVER_APP_TOKEN=...   # values come from the user, not from you
export PUSHOVER_USER_KEY=...
povr validate --user "$PUSHOVER_USER_KEY"
```

## Config file

`~/.config/pushover/pushover.conf`, TOML. It is the fallback for every role and the
only credential source that survives a daemon or launchd run, where no shell
environment is inherited. Set `PUSHOVER_CONFIG_DIR` to relocate it (the session state
file moves with it).

Write the file with your own file tools — this skill runs no file writes:

```toml
app_token     = "..."
user_key      = "..."
team_token    = "..."
api_url       = "https://api.pushover.net"
ws_url        = "wss://client.pushover.net/push"
session_store = "file" # or "keychain" to use the macOS Keychain for the session
```

All keys are optional; `api_url` and `ws_url` override the built-in endpoints and are
themselves overridden by `PUSHOVER_API_URL` and `PUSHOVER_WS_URL`. Ask the user for the
secret values, do not invent placeholders that look real. Then `chmod 600` the file:
povr warns on stderr when the file is group- or world-readable, and warns again for
unknown keys.

## Check it

```sh
povr validate --user <user-or-group-key>
povr validate --user <user-or-group-key> --device <device-name>
```

Prints `valid` or `invalid`, plus `devices:` and `licenses:` lines when the API returns
them. It resolves `app_token` itself, so a success here proves both the token and the
key. `--user` is required; validation does not fall back to the configured user key.

## Receiving

Receiving is a separate session, unrelated to the app token:

```sh
povr client login                   # interactive: email, password, 2FA code on demand
povr client register --name laptop  # 1-25 chars, letters, digits, _ and -
povr client listen                  # foreground; see the launchd section for a permanent one
```

`login` reads the password from the terminal, so it needs a TTY and cannot be driven
from a normal tool call. Ask the user to run it themselves — in Claude Code,
`! povr client login`. There is no flag to pass the email or password. Only the session
secret returned by the server is stored — in `~/.config/pushover/state.json`, or in the
macOS Keychain when `session_store = "keychain"` (see above); the password is not.
`register` must run after `login` and before `fetch` or `listen`.

`listen` posts native macOS notifications by default. For a headless or agent-readable
run, point it at the file backend instead — that backend is chosen before any platform
dispatch, so it also works on Linux, where the desktop backends refuse to start:

```sh
export PUSHOVER_NOTIFY_BACKEND=file:./povr-notifications.jsonl
povr client listen
```

Both lines belong to one shell invocation; an `export` run on its own does not survive
into the next command.

See `/povr:read` for reading what arrives.

For the fullest-fidelity macOS notifications (custom icons, sounds, levels), install the
helper bundle once. Without it, povr falls back to alerter, if installed, or osascript,
with reduced fidelity. `povr helper install` is macOS-only; it copies the staged bundle
(from the Homebrew formula's libexec, or a path given via `--from`) and re-signs it
ad hoc. Only the bare command is pre-approved, so the `--from` form asks first. From a
repository checkout you can instead run `./scripts/build-helper.sh`.

## Permanent delivery on macOS

`povr client listen` in the foreground, or backgrounded from a shell, dies with the
shell that started it. For delivery that comes back at every login, and after a reboot,
without being restarted by hand, install the launchd agent instead:

```sh
povr daemon install    # write ~/Library/LaunchAgents/net.vshuraeff.povr.listen.plist and load it
povr daemon status     # plist installed, service loaded, pid, last exit code, detail
povr daemon stop       # unload, leaving the plist in place
povr daemon start      # load the installed plist again
povr daemon restart
```

The plist runs `povr client listen` with `RunAtLoad` and `KeepAlive`, so launchd starts
it at login and restarts it when it exits. It carries no environment, which has two
consequences. Nothing it needs may come from an exported variable: `app_token` and
`user_key` have to be in the TOML config file, and the Open Client session has to be in
`state.json`, or in the Keychain when `session_store = "keychain"`. And
`PUSHOVER_NOTIFY_BACKEND` cannot be set for it, so the agent always uses the desktop
backend. A file-backend run for an agent to read stays a foreground `povr client listen`.

`povr daemon status` reports on the launchd job, not on delivery: `service loaded: true`
with a non-zero `last exit code` means launchd is restarting a job that keeps failing.
The whole family is macOS-only; elsewhere it fails with
`launchagent: not implemented on this platform`.

## Failure modes

- `missing app_token credential; checked environment variables: ...; config file "..."`
  — nothing in the resolution list above supplied that role. Export the default
  variable, or add the key to the config file.
- `environment variable "X" for app_token is unset or empty` — a `--*-env` flag named
  `X` and it is not exported. That path never falls back; export it or drop the flag.
- ``no session found: run `povr client login` and `povr client register` first`` — the
  receive half is unconfigured, or `register` was skipped after `login`.
- `notify: helper binary not found in bundle` — `PUSHOVER_NOTIFY_BACKEND=helper` was
  requested but `Povr.app` is not installed. `povr helper status` prints whether the
  bundle is installed, its path, and whether its signature is valid. Install it, or use
  `PUSHOVER_NOTIFY_BACKEND=file:<path>`.
- `notify: backend "helper" is only available on darwin` — Linux. Use the file backend.
- Password prompt fails with an ioctl error — no TTY. Hand `povr client login` to the
  user.
