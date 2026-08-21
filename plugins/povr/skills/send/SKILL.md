---
name: send
description: Send a Pushover notification with the povr CLI — plain, high-priority, or emergency messages with acknowledgement receipts, plus sounds, attachments and links. Use when the user asks to notify them, ping their phone, alert someone on Pushover, or be told when a long-running job finishes.
argument-hint: "[message text]"
allowed-tools: Bash(povr send:*), Bash(povr sounds), Bash(povr sounds:*), Bash(povr receipt:*), Bash(povr limits), Bash(povr limits:*)
---

# Send a Pushover notification

```sh
povr send "backup completed"
povr send --title "CI" --message "build 412 failed" --priority 1 --sound siren
```

The message is the positional argument, or `--message`, or stdin when the argument is
`-`. Without `--user` it goes to the configured user key; `--user` takes a
comma-separated list of user or group keys.

Credentials are never flag values: the app token and user key come from the
environment or the config file. `--encrypted` also requires a message encryption key
from `--encryption-key-env`, `PUSHOVER_ENCRYPTION_KEY`, or the `encryption_key` config
key. If a command fails to resolve one, run `/povr:setup` rather than passing a secret
on the command line.

Output is two lines, `request: <id>` and, for emergencies, `receipt: <id>`. Add
`--json` for one machine-readable object: `{"status":1,"request":"...","receipt":"..."}`.

## Flags

`--user`, `--message`, `--device`, `--title`, `--url`, `--url-title`, `--priority`,
`--retry`, `--expire`, `--callback`, `--tags`, `--sound`, `--timestamp`, `--ttl`,
`--html`, `--monospace`, `--encrypted`, `--attachment`, `--attachment-base64`,
`--attachment-type`, `--json`.

For plaintext sends, rejected before any request is made: message longer than 1024
runes, title over 250, `--url` over 512, `--url-title` over 100, `--priority` outside
-2..2, an attachment over 5242880 bytes, and both attachment flags at once. Encrypted
sends validate plaintext lengths first, but encryption expands field values and
Pushover's public docs do not specify whether server limits apply to plaintext or
ciphertext lengths.

## Priority

`--priority` runs -2 (lowest, silent) through 2 (emergency). Use 0 for ordinary
progress reports, 1 when the user should look now, 2 only for something that must be
acknowledged.

Priority 2 requires `--retry` (seconds between re-alerts, at least 30) and `--expire`
(seconds until it stops, 1 to 10800):

```sh
povr send --priority 2 --retry 60 --expire 3600 --tags disk "root filesystem full"
povr receipt status rXXXXXXXXXXXXXXXXXXXXXXXXXXXXX --json
povr receipt status rXXXXXXXXXXXXXXXXXXXXXXXXXXXXX --wait
povr receipt cancel rXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
povr receipt cancel-by-tag disk
```

`receipt status` prints `acknowledged`, `expired`, `called_back` and `cancelled` among
other fields; `--wait` polls until one of those is true, so do not run it without a
bounded expectation of how long that takes. `cancel-by-tag` cancels every unacknowledged
emergency carrying the `--tags` value. All receipt subcommands accept `--json`.

## Sound, links, attachments

```sh
povr sounds --json                                  # valid --sound names for this app
povr send --url https://example.com/build/412 --url-title "build log" "build failed"
povr send --title "CI" --attachment ./failure.png "build 412 failed"
```

`--attachment` takes a file path and uploads it as a multipart file. `--attachment-base64`
also takes a path to a file holding the raw attachment bytes — not pre-encoded base64 —
which povr reads and base64-encodes itself into the API payload; pairs with
`--attachment-type` (the MIME type is detected when omitted). Passing an already
base64-encoded file to `--attachment-base64` would double-encode it. The two attachment
flags are mutually exclusive.

`--html` and `--monospace` switch the body's rendering, `--ttl` sets how many seconds the
message survives on the user's devices, `--timestamp` overrides its displayed time as
unix seconds, and `--device` narrows delivery to one registered device name.

## Failure modes

- `missing app_token credential; checked environment variables: ...` or
  `environment variable "X" for user_key is unset or empty` — credentials are not
  configured. Run `/povr:setup`; do not work around it with a literal key.
- `retry must be at least 30 for emergency priority` /
  `expire must be between 1 and 10800 for emergency priority` — priority 2 without a
  valid re-alert window.
- `message exceeds 1024 runes` — shorten the body, or move the detail behind `--url`.
- `message limit reached; resets at <time>` — the application's quota is spent.
  `povr limits` prints the quota, what remains, and the reset timestamp.
