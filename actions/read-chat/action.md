+++
name = "read-chat"
version = "0.0.0-dev"
source = "github://ALRubinger/aileron-connector-bluebubbles/actions/read-chat@0.0.0-dev"

[[requires.connectors]]
name = "github://ALRubinger/aileron-connector-bluebubbles"
version = "0.0.0-dev"
hash = "sha256:bound-at-release"
capabilities = ["read_chat"]

[match]
intent = "read recent messages from a specific iMessage conversation"

[[execute]]
id = "read"
connector = "github://ALRubinger/aileron-connector-bluebubbles"
op = "read_chat"

[execute.inputs]
chat_guid = "${args.chat_guid}"
limit = "${args.limit}"
offset = "${args.offset}"

[[inputs]]
name = "chat_guid"
type = "string"
description = "The chat GUID, as returned by list-recent-chats in each entry's `guid` field. Example: \"iMessage;-;+15551234567\"."
required = true

[[inputs]]
name = "limit"
type = "integer"
description = "How many messages to return, most recent first. Default 50, max 200."
required = false

[[inputs]]
name = "offset"
type = "integer"
description = "How many messages to skip from the most-recent end. Default 0."
required = false
+++

# Read an iMessage Conversation

Returns recent messages from a single iMessage chat, most-recent
first. Use `list-recent-chats` first to discover the `chat_guid`.

When it fires:
- "what did Alice and I last talk about?"
- "summarize my conversation with the family group chat"
- "show me the last 20 messages with Bob"

This action is **read-only** and idempotent. The bridge reads from
the local Messages database; no remote service is contacted and no
state on the user's Mac changes.

Output is the raw BlueBubbles `/api/v1/chat/{guid}/message` envelope:
a `data` array of message objects with `guid`, `text`, `isFromMe`,
`dateCreated`, `handle` (the participant who sent it), and any
attachments metadata. The agent typically reads `text`, `isFromMe`,
and `dateCreated` to summarize the conversation.

Prerequisite: [BlueBubbles Server](https://docs.withaileron.ai/guides/setting-up-bluebubbles/)
must be running on the user's Mac.
