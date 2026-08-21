---
name: read
description: Read incoming Pushover messages with the povr CLI — fetch the queued backlog as JSON, acknowledge emergency alerts, or run the listen daemon writing notifications to a JSONL file an agent can tail. Use when the user asks what Pushover messages arrived, to check or acknowledge an emergency alert, or to watch for incoming notifications.
allowed-tools: Bash(povr client fetch), Bash(povr client fetch:*), Bash(povr client ack:*), Bash(povr client listen:*), Bash(povr client register:*)
---

# Read incoming Pushover messages

Receiving runs on an Open Client session, not on the app token used for sending. The
session is created once, interactively; everything after that is scriptable.

## One-shot fetch

```sh
povr client fetch --json
```

Prints the queued messages as a single JSON array line and then advances the server-side
cursor, so each message is returned once — capture the output before deciding what to do
with it. Each element carries `id`, `date`, `priority`, `app`, `title`, `message`,
`sound`, `url`, `url_title`, `acked` and `receipt` among other fields. Without `--json`
the same messages print as an aligned table with the header
`ID DATE PRIORITY APP TITLE MESSAGE`. An empty queue prints an empty JSON array `[]` with
`--json`, or the bare header without it, and exits 0.

Acknowledge an emergency message so its re-alerts stop, using the receipt from the
message:

```sh
povr client ack rXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
```

## Watching continuously

`povr client listen` keeps the websocket open, delivers the backlog first, and posts a
notification per message. Point it at the file backend to get machine-readable output
instead of desktop notifications; that backend is selected before any platform check, so
it is also the only one that runs on Linux:

```sh
PUSHOVER_NOTIFY_BACKEND=file:./povr-notifications.jsonl povr client listen
```

The daemon runs in the foreground until it is stopped — start it in the background and
read the file separately, never as a blocking foreground call. Each line is one JSON
object: `id`, `title`, `body`, `appName`, `priority`, `url`, `urlTitle`, `threadID`,
`iconPath`, `attachmentPath`, `soundName`, `soundPath`, `level`
(`passive`, `active` or `timeSensitive`) and `soundSuppressed`. The file is appended to
and created if missing, so a tail from the end sees only new messages.

Priority-2 messages re-alert locally until acknowledged or until a hard cap of 3 hours
from first delivery is reached, whichever comes first.
`povr client listen --emergency-realert <seconds>` sets the interval; the default is 60
and values below 30 are rejected. Messages that could not be delivered to the backend are
appended to `~/.config/pushover/deadletter.jsonl`.

## First-time session

```sh
povr client login                   # email, password, 2FA code when the server asks
povr client register --name laptop  # 1-25 chars, letters, digits, _ and -
```

`login` reads the password from the terminal and needs a TTY, so it cannot be driven
from a tool call and takes no email or password flag. Ask the user to run it — in Claude
Code, `! povr client login` — then run `register` yourself if no device exists yet.
`/povr:setup` covers the rest of the configuration.

## Failure modes

- `no session found: run 'povr client login' and 'povr client register' first` —
  no session, or `register` was never run. `fetch` and `listen` both need the device id,
  so `login` alone is not enough.
- The password prompt fails with an ioctl error — no TTY. Hand `povr client login` to
  the user.
- `notify: helper binary not found in bundle` or
  `notify: backend "helper" is only available on darwin` — `listen` was asked for a
  desktop backend that is unavailable. Use `PUSHOVER_NOTIFY_BACKEND=file:<path>`.
- `--emergency-realert must be at least 30 seconds` — raise the interval.
- Messages that already appeared in an earlier `fetch` do not come back: the cursor
  moved. Re-read the output you captured, or use `listen` with the file backend for a
  durable log.
