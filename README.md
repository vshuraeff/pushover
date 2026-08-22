# povr

`povr` is an unofficial command-line client for the Pushover service, written in Go.
It is an independent Open Client implementation and is **not affiliated with, endorsed
by, or supported by Pushover or its makers**. It ships no Pushover branding or
artwork. Report problems with `povr` here, not to Pushover support.

Two halves:

- **Send** — full coverage of the public Pushover HTTP API: messages, attachments,
  emergency receipts, user/group validation, groups, glances, sounds, app limits,
  licensing, teams, subscription migration.
- **Receive** — an Open Client session (login, device registration, message fetch,
  acknowledgement) and a `listen` daemon that turns pushed messages into native macOS
  notifications.

## Requirements

- Go 1.26 or newer to build.
- macOS or Linux; the session store uses `flock`, so Windows is not supported.
  Notifications are macOS-only — on Linux the notification backend is a stub and
  `povr client listen` refuses to start, while sending is unaffected (see
  [Known limits](#known-limits)).

## Install

```sh
go build ./cmd/povr        # or: make build
```

### Install via Homebrew

```sh
brew tap vshuraeff/povr && brew install povr
# or: brew install vshuraeff/povr/povr
```

Run `povr helper install` after installing or upgrading to enable native macOS notifications.

`make` targets: `build`, `test` (`go test ./...`), `vet`, `e2e`
(`go test -tags e2e ./...`), `fmt`, `tidy`, `clean`.

### macOS notification helper

Native notifications with icons, sounds and interruption levels are delivered through
a small `Povr.app` bundle. Build and install it once:

```sh
./scripts/build-helper.sh
```

The script builds `cmd/povr-helper`, assembles `Povr.app`, signs it **ad hoc**
(`codesign -s -`) and installs it to `~/Library/Application Support/povr/Povr.app`.
The first notification triggers the usual one-time macOS permission prompt; grant it,
otherwise nothing is displayed. By default the bundle is signed without entitlements —
a restricted entitlement under ad-hoc signing gets the helper killed by AMFI at launch.
To opt in (with a provisioning profile), set `POVR_HELPER_ENTITLEMENTS` to a plist
path — see [Known limits](#known-limits).

Backend selection on macOS, in order: the installed helper bundle, then `alerter` if
it is on `PATH`, then AppleScript. Override with `PUSHOVER_NOTIFY_BACKEND`:
`helper`, `alerter`, `osascript`, or `file:<path>`.

## Credentials

Credential-role values are never passed as command-line values: the `--*-env` flags
below name an environment variable, never its contents. Each command resolves the roles
it needs — `app_token`, `user_key`, `team_token`, `encryption_key` — from the environment
first, then from the config file. The one secret that does take a literal flag value is
`povr team add-user --password`; omit it and no password is sent, which also keeps it
out of your shell history and the process list.

Resolution order per role:

1. The variable named by a `--*-env` flag, if one is given. If that variable is unset
   or empty the command fails; there is no fallback.
2. The role's default variable: `PUSHOVER_APP_TOKEN`, `PUSHOVER_USER_KEY`,
   `PUSHOVER_TEAM_TOKEN`, `PUSHOVER_ENCRYPTION_KEY`.
3. `PUSHOVER_KEY`, but only for the command's *primary* role. The primary role is
   `app_token` for every command except `povr team ...`, where it is `team_token`.
4. The config file (see below). Environment values always win over the file.

The `--*-env` flags name an environment **variable**, not a value:

| Flag | Role |
| --- | --- |
| `--key-env` | the command's primary role |
| `--token-env` | `app_token` |
| `--user-env` | `user_key` |
| `--team-env` | `team_token` |
| `--encryption-key-env` | `encryption_key` |

Role-specific flags take precedence over `--key-env` for the primary role.

```sh
export WORK_TOKEN=a0000000000000000000000000000
povr send --token-env WORK_TOKEN "deploy finished"
```

### Config file

`~/.config/pushover/pushover.conf`, TOML:

```toml
app_token      = "a0000000000000000000000000000"
user_key       = "u0000000000000000000000000000"
team_token     = "t0000000000000000000000000000"
encryption_key = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
api_url        = "https://api.pushover.net"
ws_url         = "wss://client.pushover.net/push"
session_store  = "file" # or "keychain" (macOS only)
```

All keys are optional. `api_url` and `ws_url` override the built-in endpoints and are
themselves overridden by `PUSHOVER_API_URL` and `PUSHOVER_WS_URL`. Unknown keys and
group-readable permissions produce a warning on stderr; keep the file at `chmod 600`.
The file matters most for daemon or launchd runs, where no environment is inherited.

### Session state

`povr client login` and `povr client register` write
`~/.config/pushover/state.json` (mode `0600`; the directory gets mode `0700` when
`povr` creates it, a pre-existing one keeps its own permissions) holding the device id,
device name, session secret and last processed message id. **The account password is
never stored** — only the secret the server returns. Set `PUSHOVER_CONFIG_DIR` to move
both the config file and the state file elsewhere. Set `session_store = "keychain"`
(or `PUSHOVER_SESSION_STORE=keychain`) to keep the session in the macOS Keychain
instead of `state.json`; an unrecognized value makes all client commands fail closed.

## Command reference

Placeholder keys below are obviously fake.

### Messages

```sh
povr send "backup completed"
```

Reads the message from the positional argument, from stdin when the argument is `-`,
or from `--message`. Without `--user` the message goes to the resolved `user_key`.

Flags: `--user` (comma-separated recipient or group keys), `--message`, `--device`,
`--title`, `--url`, `--url-title`, `--priority`, `--retry`, `--expire`, `--callback`,
`--tags`, `--sound`, `--timestamp`, `--ttl`, `--html`, `--monospace`, `--encrypted`,
`--attachment`, `--attachment-base64`, `--attachment-type`, `--json`.

`--encrypted` encrypts `message`, `title`, `url`, and `url_title` client-side with
AES-256-CBC and HMAC-SHA256 using [Pushover's documented e2ee scheme](https://pushover.net/api#e2ee).
Generate a 64-character hex key with `openssl rand -hex 32` and configure it through
`--encryption-key-env`, `PUSHOVER_ENCRYPTION_KEY`, or `encryption_key` in the config
file. `--encrypted` fails without sending when no encryption key credential resolves.

```sh
# attachment; 5242880 bytes maximum, --attachment and --attachment-base64 are exclusive
povr send --title "CI" --attachment ./failure.png "build 412 failed"

# emergency, prints the receipt id
povr send --priority 2 --retry 60 --expire 3600 --tags disk "root filesystem full"
```

### Receipts

```sh
povr receipt status rXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
povr receipt status rXXXXXXXXXXXXXXXXXXXXXXXXXXXXX --wait --json
povr receipt cancel rXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
povr receipt cancel-by-tag disk
```

`--wait` polls until the receipt is acknowledged, expired, called back or cancelled.
All three subcommands take `--json`.

### Users

```sh
povr validate --user u0000000000000000000000000000 --device iphone
```

Prints `valid` or `invalid`, plus the device and license lists when the API returns
them.

### Groups

`povr group` accepts `--json` for all its subcommands and uses the standard
endpoint configuration.

```sh
povr group create oncall
povr group list
povr group show g0000000000000000000000000000
povr group add-user g0000000000000000000000000000 u0000000000000000000000000000 \
  --device iphone --memo "primary"
povr group remove-user g0000000000000000000000000000 u0000000000000000000000000000
povr group disable-user g0000000000000000000000000000 u0000000000000000000000000000
povr group enable-user g0000000000000000000000000000 u0000000000000000000000000000
povr group rename g0000000000000000000000000000 oncall-eu
```

### Glances

```sh
povr glance --user u0000000000000000000000000000 --title Queue --text "12 jobs" --count 12
```

Only the fields you pass are sent. Also accepts `--device`, `--subtext`, `--percent`.

### Sounds and limits

```sh
povr sounds --json
povr limits
```

`limits` prints the application quota and the values from the rate-limit response
headers.

### Licensing

```sh
povr license credits
povr license assign --user u0000000000000000000000000000 --os Desktop
```

Exactly one of `--user` or `--email` is required. Assignment spends a credit and is
irreversible, so it asks for confirmation; `--yes` skips the prompt and is mandatory
when stdin is not a terminal.

### Teams

Team commands use the `team_token` role.

```sh
povr team list
povr team add-user --email dev@example.com --name "Dev" --admin --instant
povr team revoke-invitation --email dev@example.com
povr team remove-user --email dev@example.com
```

`add-user` also accepts `--password` and `--group`.

### Subscriptions

```sh
povr subscription migrate --subscription sXXXXXXXXXXXXXXXXXXXXXXXXXXXXX \
  --user u0000000000000000000000000000 --device-name laptop --sound cosmic
```

### Daemon

Manage the launchd agent that runs `povr client listen`.

```sh
povr daemon install
povr daemon uninstall
povr daemon start
povr daemon stop
povr daemon restart
povr daemon status --json
```

`install` installs and loads the launchd agent for `client listen`.
`uninstall` unloads and removes the launchd agent.
`start` loads the installed launchd agent.
`stop` unloads the launchd agent.
`restart` restarts the launchd agent.
`status` shows launchd agent status and supports `--json`.

### Version

```sh
povr version
```

## Receiving messages

```sh
povr client login                  # email, password, 2FA code when the server asks
povr client register --name laptop # 1-25 characters, letters, digits, _ and -
povr client listen
```

`login` posts to `/1/users/login.json`; when the server answers `412` it prompts once for
a two-factor code and retries. `register` posts to `/1/devices.json` with `os=O`. Both
write to the configured [session store](#session-state). `listen` first fetches the
queued backlog, then opens the websocket and sends `login:<device id>:<secret>` for live
delivery; it downloads message icons and sounds into the user cache directory
(`povr/icons`, `povr/sounds`) and posts a native notification per message. Messages that
cannot be delivered are appended to `~/.config/pushover/deadletter.jsonl`.

Per Pushover's Open Client API documentation an Open Client device (`os=O`) counts as the
**Desktop** platform, so a Desktop user license covers a device registered by `povr`
(see [Licensing](#licensing)).

The server then drives the connection with single-character frames:

| Frame | Meaning |
| --- | --- |
| `#` | keepalive |
| `!` | new messages — fetch `/1/messages.json`, then confirm with `update_highest_message` |
| `R` | reconnect immediately |
| `E` | session permanently rejected; `listen` exits |
| `A` | session taken over by another connection; `listen` exits without reconnecting |

One-shot alternatives, useful for scripts:

```sh
povr client fetch --json            # prints queued messages and advances the cursor
povr client ack rXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
```

### Emergency messages

Priority-2 messages keep re-alerting locally while the receipt is unacknowledged.
`povr client listen --emergency-realert <seconds>` sets the interval (default 60,
minimum 30). Re-alerting for a given receipt stops when the message is acknowledged —
`povr client ack <receipt>`, or an acknowledgement from any other device — and in any
case three hours after the daemon first saw it.

### Priority mapping

| Pushover priority | macOS interruption level | Notes |
| --- | --- | --- |
| -2 (lowest) | `passive` | sound suppressed |
| -1 (low) | `passive` | sound suppressed |
| 0 (normal) | `active` | |
| 1 (high) | `timeSensitive` | |
| 2 (emergency) | `timeSensitive` | plus the local re-alert loop above |

### Known limits

- Critical alerts (the level that pierces Focus and silent mode) require an entitlement
  granted by Apple to a registered developer account. `povr` cannot request it, so that
  level is not available.
- `timeSensitive` delivery needs the time-sensitive entitlement, which the default
  ad-hoc build does not request (a restricted entitlement under ad-hoc signing gets
  the helper killed by AMFI at launch). The helper falls back to `active` and says so.
- Linux notifications are not implemented. `povr client listen` fails at startup with
  `backend not implemented on this platform`. Sending is unaffected.

## Development

### Mock server

`cmd/povr-mock` runs an in-process Pushover stand-in — REST endpoints plus the Open
Client websocket protocol — and prints its URLs as a single JSON line on startup:

```sh
go run ./cmd/povr-mock -addr 127.0.0.1:0
{"api_url":"http://127.0.0.1:53412","ws_url":"ws://127.0.0.1:53412/push"}
```

Point the client at it and, for headless runs, record notifications to a file instead
of the desktop:

```sh
export PUSHOVER_API_URL=http://127.0.0.1:53412
export PUSHOVER_WS_URL=ws://127.0.0.1:53412/push
export PUSHOVER_NOTIFY_BACKEND=file:/tmp/povr-notifications.jsonl
```

The file backend appends one JSON record per notification, including the resolved
level and whether sound was suppressed.

### Tests

```sh
make test    # go test ./...
make e2e     # go test -tags e2e ./...
```

The end-to-end suite runs behind the `e2e` build tag against the mock server, so a
plain `make test` skips it.

## Trademarks

Pushover is a trademark of its owner. This project is an independent Open Client and
uses the name only to identify the service it talks to.
