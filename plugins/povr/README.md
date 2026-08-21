# povr plugin for Claude Code

Three skills that let an agent drive the `povr` Pushover CLI:

- `/povr:setup` — credential resolution, the TOML config file, and the Open Client
  session used for receiving.
- `/povr:send` — notifications, priorities, emergency messages and receipts.
- `/povr:read` — fetching queued messages, acknowledging emergencies, and running the
  listen daemon with a JSONL notification log.

The skills assume `povr` is on `PATH`; build it from this repository with
`go build ./cmd/povr`.

## Install

```
/plugin marketplace add vshuraeff/pushover
/plugin install povr@povr
```

For development against a local checkout, add the marketplace by path instead — the
repository root is the marketplace:

```
/plugin marketplace add ./pushover
```

## Headless agents

Enable the plugin in `.claude/settings.json` so it loads without the interactive
installer:

```json
{
  "extraKnownMarketplaces": {
    "povr": {
      "source": {
        "source": "github",
        "repo": "vshuraeff/pushover"
      }
    }
  },
  "enabledPlugins": {
    "povr@povr": true
  }
}
```
