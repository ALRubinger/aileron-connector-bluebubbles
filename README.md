# aileron-connector-bluebubbles

iMessage connector for [Aileron](https://docs.withaileron.ai). Gives
your agent read and send access to your iMessage history on macOS,
without granting the agent itself any platform permissions.

## How it works

Aileron does not touch Messages.app directly. Instead this connector
talks to a local [BlueBubbles](https://bluebubbles.app/) server
running on your Mac, which holds the macOS Full Disk Access and
Automation permissions and translates HTTP requests into reads of the
Messages database and AppleScript-driven sends.

```
agent ──→ Aileron daemon ──HTTP──→ BlueBubbles ──→ Messages.app
                                   (localhost:1234)
```

The user grants the platform permissions to BlueBubbles (purpose-built
for this) instead of to Aileron's daemon. The decision rationale lives
at [issue #514](https://github.com/ALRubinger/aileron/issues/514) in
the Aileron repo.

## Prerequisite

[Set up BlueBubbles Server first.](https://docs.withaileron.ai/guides/setting-up-bluebubbles/)
About five minutes, one-time per Mac. Without BlueBubbles running, the
connector returns `bridge_unreachable` with setup pointers.

## Install

```sh
# Trust this publisher once per machine (fetches keys/publisher.pub
# from this repo's default branch).
aileron keyring trust github://ALRubinger/aileron-connector-bluebubbles

# Install the connector at a specific tag.
aileron connector install github://ALRubinger/aileron-connector-bluebubbles@0.0.1

# Install the actions you want exposed to the agent.
aileron action add github://ALRubinger/aileron-connector-bluebubbles/actions/list-recent-chats@0.0.1
aileron action add github://ALRubinger/aileron-connector-bluebubbles/actions/read-chat@0.0.1
aileron action add github://ALRubinger/aileron-connector-bluebubbles/actions/send-message@0.0.1

# Bind your BlueBubbles server password — the runtime stores it
# encrypted in the local vault and injects it host-side at the
# network boundary; the connector code never sees the bytes.
aileron binding setup github://ALRubinger/aileron-connector-bluebubbles
```

Then launch:

```sh
aileron launch claude
```

## Operations

| Op | Endpoint | Idempotent | Approval gate |
|---|---|---|---|
| `list_recent_chats` | `GET /api/v1/chat/query` | yes | no |
| `read_chat` | `GET /api/v1/chat/{guid}/message` | yes | no |
| `send_message` | `POST /api/v1/message/text` | **no** | **yes** |

`send_message` is gated by per-call user approval ([ADR-0009](https://docs.withaileron.ai/adr/0009-user-channel))
and is not idempotent. The Aileron runtime asks the user via the
launch-comms channel before BlueBubbles dispatches; on denial nothing
is sent.

## Error classes

The connector emits structured errors per [ADR-0010](https://docs.withaileron.ai/adr/0010-failure-handling):

| Class | When |
|---|---|
| `bridge_unreachable` | BlueBubbles is not running on `localhost:1234`. Message tells the user to open Applications and relaunch BlueBubbles Server, plus links the setup guide. |
| `unauthorized` | BlueBubbles returned 401 or 403. The bound password does not match what BlueBubbles expects. Message tells the user to re-run `aileron binding setup`. |
| `external_api_error` | BlueBubbles returned a non-2xx status that's not 401/403. Body is included for the agent and the audit log. |
| `connector_runtime_error` | Malformed input, unparseable response, or a missing required arg. |

## Build from source

```sh
task build       # produces connector.wasm
task test        # unit tests + wasip1 build smoke test
task pack        # builds the local tarball (offline signing path)
task pack:hash   # prints the canonical-hash the release pipeline computes
```

The release pipeline is composite actions from
[aileron-actions](https://github.com/ALRubinger/aileron-actions); each
`uses:` in `.github/workflows/release.yml` is SHA-pinned for
supply-chain trust. Pushing a `vX.Y.Z` tag triggers the full
build → sign → publish chain.

## Capability declarations

The connector manifest declares:

- **`[capabilities.network] hosts = ["localhost:1234"]`** — only the
  BlueBubbles default port. Anything else is denied at the sandbox
  boundary.
- **`[capabilities.credential] kind = "api_key"`** — the BlueBubbles
  server password. The runtime injects it as
  `Authorization: Bearer <password>` host-side; the connector never
  sees the bytes.

If you run BlueBubbles on a non-default port, you'll need to fork the
connector and update `[capabilities.network]` in `connector/manifest.toml`
to match. A per-binding URL override is on the v3 roadmap.

## See also

- [Setting up BlueBubbles for Aileron](https://docs.withaileron.ai/guides/setting-up-bluebubbles/)
- [ADR-0002: Connector Model](https://docs.withaileron.ai/adr/0002-connector-model)
- [ADR-0009: User Channel and Approval Surfaces](https://docs.withaileron.ai/adr/0009-user-channel)
- [BlueBubbles documentation](https://bluebubbles.app/docs/)

## License

Apache 2.0. See [LICENSE](LICENSE).
