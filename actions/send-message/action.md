+++
name = "send-message"
version = "0.0.0-dev"
source = "github://ALRubinger/aileron-connector-bluebubbles/actions/send-message@0.0.0-dev"

[[requires.connectors]]
name = "github://ALRubinger/aileron-connector-bluebubbles"
version = "0.0.0-dev"
hash = "sha256:bound-at-release"
capabilities = ["send_message"]

[match]
intent = "send an iMessage to a specific conversation on behalf of the user"

[[execute]]
id = "send"
connector = "github://ALRubinger/aileron-connector-bluebubbles"
op = "send_message"
idempotent = false

[execute.inputs]
chat_guid = "${args.chat_guid}"
message = "${args.message}"

# Per-call approval gate. The runtime asks the user via the
# launch-comms channel before BlueBubbles dispatches to Messages.app
# (per ADR-0009 — agent in trust path for irreversible actions).
# On approval the connector runs; on denial the connector is never
# invoked, no message leaves, and the runtime audit-logs the deny.
# send_message is gated because dispatched iMessages are not
# recoverable — once delivered, there is no Drafts-folder equivalent
# to retract.
[approval]
required = true

[[inputs]]
name = "chat_guid"
type = "string"
description = "The chat GUID to send to, as returned by list-recent-chats. Example: \"iMessage;-;+15551234567\"."
required = true

[[inputs]]
name = "message"
type = "string"
description = "The message body to send. The user will be asked to approve this exact text before BlueBubbles dispatches it to Messages.app."
required = true
+++

# Send an iMessage

Sends a text iMessage to an existing chat via the user's Mac.

When it fires:
- "tell Alice I'll be ten minutes late"
- "let the family group know I'm on my way"
- "respond to Bob with a thumbs up"

This action is **gated on per-call user approval**. When the agent
calls `send_message`, the Aileron runtime pauses the call and asks
the user to approve via the launch-comms channel (CLI prompt or the
webapp `/approvals` surface). BlueBubbles is not contacted until
approval is granted. On denial the call returns an error to the
agent and is recorded in the audit log; no iMessage is dispatched.

This action writes to your iMessage (sends a message). It is **not
idempotent** — invoking it twice sends two iMessages. The runtime's
retry layer is configured to honor that and will not double-send on
transient failure.

For unattended flows where the user would prefer to review-and-send
manually, use the Messages app directly. There is no Drafts-folder
analog for iMessage; the choice is approve-then-send (this action)
or compose-in-Messages-yourself.

Prerequisite: [BlueBubbles Server](https://docs.withaileron.ai/guides/setting-up-bluebubbles/)
must be running on the user's Mac and granted the macOS Automation
permission for Messages (covered by step 2 of the setup guide).

The connector runs in the Aileron WASM sandbox with
`[capabilities.network]` restricted to `localhost:1234`, and the
BlueBubbles server password is injected host-side at the network
boundary — the connector code never sees the password bytes.
