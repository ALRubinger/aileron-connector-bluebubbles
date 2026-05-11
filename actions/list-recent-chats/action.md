+++
name = "list-recent-chats"
# `version` and the `0.0.0-dev` markers in `source` and the
# `[[requires.connectors]]` block are placeholders. CI substitutes
# them with the real version (from the pushed tag) into a build copy
# of this manifest before signing and packing. Source stays template;
# only the published tarball carries the real version.
version = "0.0.0-dev"
source = "github://ALRubinger/aileron-connector-bluebubbles/actions/list-recent-chats@0.0.0-dev"

[[requires.connectors]]
name = "github://ALRubinger/aileron-connector-bluebubbles"
version = "0.0.0-dev"
# `hash` is the connector tarball's content-addressed identity per
# ADR-0002. CI substitutes this placeholder with the real hash at
# release time (see .github/workflows/release.yml).
hash = "sha256:bound-at-release"
capabilities = ["list_recent_chats"]

[match]
intent = "list iMessage conversations, ordered most-recent first"

[[execute]]
id = "list"
connector = "github://ALRubinger/aileron-connector-bluebubbles"
op = "list_recent_chats"

[execute.inputs]
limit = "${args.limit}"
offset = "${args.offset}"

[[inputs]]
name = "limit"
type = "integer"
description = "How many chats to return. Default 25, max 100."
required = false

[[inputs]]
name = "offset"
type = "integer"
description = "How many chats to skip from the most-recent end. Use for paging through history. Default 0."
required = false
+++

# List Recent iMessage Conversations

Returns a list of iMessage chats from the user's Mac, ordered with the
most recent activity first. Each chat carries a `guid` you can pass to
`read-chat` to fetch the messages inside it.

When it fires:
- "what iMessage conversations do I have going?"
- "show me my recent texts"
- "list the chats I've replied to today"

This action is **read-only** and idempotent. The agent can call it
repeatedly without side effects; the BlueBubbles bridge reads from
the local Messages database without modifying anything.

Output is the raw BlueBubbles `/api/v1/chat/query` envelope: a `data`
array of chat objects each containing `guid`, `displayName` (when
the chat has a name; null for one-on-one chats), `lastMessage`
(snippet of the most recent message), `lastMessageTimestamp`, and
`participants`.

Prerequisite: [BlueBubbles Server](https://docs.withaileron.ai/guides/setting-up-bluebubbles/)
must be running on the user's Mac. If the bridge is unreachable the
action returns a `bridge_unreachable` error with setup instructions.
