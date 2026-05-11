// Package main is the WASM source for aileron-connector-bluebubbles. It
// targets Go's native WASI Preview 1 (`GOOS=wasip1 GOARCH=wasm`) and
// calls into Aileron's host-import ABI for outbound HTTP and
// credential mediation.
//
// Build:
//
//	cd connector && GOOS=wasip1 GOARCH=wasm go build -trimpath \
//	  -ldflags="-s -w" -o ../connector.wasm .
//
// Or via Taskfile from the repo root:
//
//	task build
//
// I/O contract (stdin → stdout JSON):
//
//	{"op": "list_recent_chats", "args": {"limit": 25}}
//	  → {"output": {"chats": [...]}}
//
//	{"op": "read_chat",
//	 "args": {"chat_guid": "iMessage;-;+15551234567", "limit": 50}}
//	  → {"output": {"messages": [...]}}
//
//	{"op": "send_message",
//	 "args": {"chat_guid": "iMessage;-;+15551234567",
//	          "message": "On my way."}}
//	  → {"output": {"guid": "...", ...}}
//
//	{"error": {"class": "...", "message": "..."}}  on failure
//
// All outbound HTTP targets localhost:1234, where BlueBubbles Server
// listens. The user's BlueBubbles password is bound as an api_key
// credential; the runtime sets `Authorization: Bearer <password>`
// host-side when an outbound request marks itself as
// `credential: "api_key"`. The connector never holds the password.
//
// Errors:
//   - bridge_unreachable: BlueBubbles is not running or not reachable
//     on localhost:1234. The connector emits this class so the
//     daemon surfaces a clear "open Applications and relaunch
//     BlueBubbles Server" message, rather than the agent seeing an
//     opaque HTTP failure. The setup guide at
//     https://docs.withaileron.ai/guides/setting-up-bluebubbles/
//     covers the prerequisites.
//   - unauthorized: BlueBubbles responded 401. The bound password
//     does not match what BlueBubbles expects. Run
//     `aileron binding setup github://ALRubinger/aileron-connector-bluebubbles`
//     to re-enter the password.
//   - external_api_error: BlueBubbles returned a non-2xx status that
//     is not 401. Body is included for the agent and the user.
//   - connector_runtime_error: malformed input, unparseable response,
//     missing required arg.
//
// Idempotency: read ops (list_recent_chats, read_chat) are
// idempotent by their HTTP shape (GET). send_message is NOT
// idempotent — calling it twice sends two iMessages. The action
// manifest for send-message MUST set [[execute]].idempotent = false
// so the runtime's retry layer (ADR-0010) does not double-send, and
// MUST gate on per-call user approval ([approval].required = true)
// since dispatched iMessages are not recoverable.
//
//go:build wasip1

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"unsafe"
)

//go:wasmimport aileron_host log
//go:noescape
func hostLog(levelPtr unsafe.Pointer, levelLen uint32, msgPtr unsafe.Pointer, msgLen uint32)

//go:wasmimport aileron_host http_request
//go:noescape
func hostHTTPRequest(reqPtr unsafe.Pointer, reqLen uint32) int32

//go:wasmimport aileron_host http_response_size
//go:noescape
func hostHTTPResponseSize() int32

//go:wasmimport aileron_host http_response_status
//go:noescape
func hostHTTPResponseStatus() int32

//go:wasmimport aileron_host http_response_read
//go:noescape
func hostHTTPResponseRead(dstPtr unsafe.Pointer, dstLen uint32) int32

// _emptyPtrSentinel keeps the address of an empty byte slice valid;
// Go can't take the address of an empty slice's first element directly.
var _emptyPtrSentinel = [1]byte{}

func ptr(b []byte) unsafe.Pointer {
	if len(b) == 0 {
		return unsafe.Pointer(&_emptyPtrSentinel[0])
	}
	return unsafe.Pointer(&b[0])
}

func aileronLog(level, message string) {
	lb := []byte(level)
	mb := []byte(message)
	hostLog(ptr(lb), uint32(len(lb)), ptr(mb), uint32(len(mb)))
}

// bluebubblesBase is the local BlueBubbles Server URL. Matches the
// [capabilities.network] declaration in manifest.toml; changing one
// without the other is a release-blocking validation error caught
// by the runtime gate.
const bluebubblesBase = "http://localhost:1234"

type input struct {
	Op   string         `json:"op"`
	Args map[string]any `json:"args"`
}

type output struct {
	Output map[string]any `json:"output,omitempty"`
	Error  *outputError   `json:"error,omitempty"`
}

type outputError struct {
	Class   string `json:"class"`
	Message string `json:"message"`
}

func main() {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		writeError("connector_runtime_error", "read_stdin: "+err.Error())
		os.Exit(1)
	}
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		writeError("connector_runtime_error", "parse_input: "+err.Error())
		os.Exit(1)
	}

	switch in.Op {
	case "list_recent_chats":
		listRecentChats(in.Args)
	case "read_chat":
		readChat(in.Args)
	case "send_message":
		sendMessage(in.Args)
	default:
		writeError("connector_runtime_error", "unknown op: "+in.Op)
		os.Exit(1)
	}
}

// listRecentChats fetches the list of chats from BlueBubbles, ordered
// most-recent-activity first. Each entry is the BlueBubbles chat
// envelope: guid, displayName, lastMessage, lastMessageTimestamp,
// participants.
//
//	GET /api/v1/chat/query?limit={n}&offset={o}&sort=lastmessage
//
// Args:
//
//	limit   (number, optional) — page size; default 25, max 100.
//	offset  (number, optional) — page offset for pagination; default 0.
func listRecentChats(args map[string]any) {
	limit := readBoundedInt(args, "limit", 25, 100)
	offset := readBoundedInt(args, "offset", 0, 10000)

	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	q.Set("sort", "lastmessage")

	body, status, err := bbGet("/api/v1/chat/query?" + q.Encode())
	if err != nil {
		writeBridgeError(err)
		return
	}
	if !okStatus(status, body) {
		return
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		writeError("connector_runtime_error", "list_recent_chats: parse: "+err.Error())
		return
	}
	writeOutput(parsed)
}

// readChat fetches recent messages for a chat, most-recent first.
//
//	GET /api/v1/chat/{guid}/message?limit={n}&offset={o}&sort=desc
//
// Args:
//
//	chat_guid  (string, required) — the chat GUID as returned by
//	           list_recent_chats (e.g. "iMessage;-;+15551234567" or
//	           "iMessage;+;chat0000000123456789").
//	limit      (number, optional) — page size; default 50, max 200.
//	offset     (number, optional) — page offset; default 0.
func readChat(args map[string]any) {
	guid, _ := args["chat_guid"].(string)
	if guid == "" {
		writeError("connector_runtime_error", "read_chat: chat_guid is required")
		return
	}
	limit := readBoundedInt(args, "limit", 50, 200)
	offset := readBoundedInt(args, "offset", 0, 10000)

	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	q.Set("sort", "desc")

	path := "/api/v1/chat/" + url.PathEscape(guid) + "/message?" + q.Encode()
	body, status, err := bbGet(path)
	if err != nil {
		writeBridgeError(err)
		return
	}
	if !okStatus(status, body) {
		return
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		writeError("connector_runtime_error", "read_chat: parse: "+err.Error())
		return
	}
	writeOutput(parsed)
}

// sendMessage sends a text iMessage to an existing chat via
// BlueBubbles.
//
//	POST /api/v1/message/text
//	Body: {"chatGuid": "...", "message": "...", "method": "apple-script"}
//
// Args:
//
//	chat_guid  (string, required) — the target chat's GUID.
//	message    (string, required) — the message body to send.
//
// `method=apple-script` is BlueBubbles' default send mode and is the
// most compatible across macOS versions. The alternative
// `private-api` route requires extra setup the user has not been
// told to perform; we leave it off for v0.0.1.
//
// NOT idempotent. Action manifest MUST set
// [[execute]].idempotent = false and gate on per-call user approval.
func sendMessage(args map[string]any) {
	guid, _ := args["chat_guid"].(string)
	message, _ := args["message"].(string)
	if guid == "" || message == "" {
		writeError("connector_runtime_error", "send_message: chat_guid and message are required")
		return
	}

	reqBody, err := json.Marshal(map[string]any{
		"chatGuid": guid,
		"message":  message,
		"method":   "apple-script",
	})
	if err != nil {
		writeError("connector_runtime_error", "send_message: encode: "+err.Error())
		return
	}

	body, status, err := bbPostJSON("/api/v1/message/text", reqBody)
	if err != nil {
		writeBridgeError(err)
		return
	}
	if !okStatus(status, body) {
		return
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		writeError("connector_runtime_error", "send_message: parse: "+err.Error())
		return
	}
	writeOutput(parsed)
}

// bbGet issues an authenticated GET against the BlueBubbles base URL.
// The runtime injects the BlueBubbles password as
// `Authorization: Bearer <password>` host-side; the connector never
// sees the bytes.
func bbGet(path string) ([]byte, int, error) {
	return bbRequest("GET", path, nil)
}

// bbPostJSON issues an authenticated POST with a JSON body. Same
// credential injection rules as bbGet.
func bbPostJSON(path string, body []byte) ([]byte, int, error) {
	return bbRequest("POST", path, body)
}

// bbRequest is the shared host-call helper. Builds the envelope,
// invokes aileron_host.http_request, reads the response.
//
// Returns (body, status, err). The host's structured *Error is on
// per-call state when rc != 0; this function returns a generic Go
// error so the caller can decide whether to map it to
// bridge_unreachable or pass through.
func bbRequest(method, path string, body []byte) ([]byte, int, error) {
	headers := map[string]string{"Accept": "application/json"}
	if body != nil {
		headers["Content-Type"] = "application/json"
	}
	envelope := map[string]any{
		"method":     method,
		"url":        bluebubblesBase + path,
		"headers":    headers,
		"credential": "api_key",
	}
	if body != nil {
		envelope["body"] = string(body)
	}
	req, err := json.Marshal(envelope)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal request: %w", err)
	}
	rc := hostHTTPRequest(ptr(req), uint32(len(req)))
	if rc != 0 {
		return nil, 0, fmt.Errorf("http_request denied or failed (rc=%d)", rc)
	}
	size := hostHTTPResponseSize()
	if size < 0 {
		return nil, 0, fmt.Errorf("http_response_size returned %d", size)
	}
	resp := make([]byte, size)
	if size > 0 {
		n := hostHTTPResponseRead(ptr(resp), uint32(size))
		if n < 0 {
			return nil, 0, fmt.Errorf("http_response_read returned %d", n)
		}
		resp = resp[:n]
	}
	return resp, int(hostHTTPResponseStatus()), nil
}

// writeBridgeError translates a transport-level failure into a
// user-friendly bridge_unreachable error. Triggered when
// hostHTTPRequest returns a non-zero status code, which typically
// means BlueBubbles is not running on localhost:1234.
//
// The user-facing language matches the troubleshooting block in the
// setup guide at
// https://docs.withaileron.ai/guides/setting-up-bluebubbles/.
func writeBridgeError(err error) {
	aileronLog("error", err.Error())
	writeError("bridge_unreachable",
		"Can't reach BlueBubbles Server on localhost:1234. "+
			"Open Applications and relaunch BlueBubbles Server. "+
			"If you haven't installed it yet, follow the setup guide at "+
			"https://docs.withaileron.ai/guides/setting-up-bluebubbles/.")
}

// okStatus checks the HTTP status. Non-2xx responses are translated
// into a structured error and the function returns false. 401 maps
// to `unauthorized` with a setup hint; everything else 4xx/5xx maps
// to `external_api_error` with the body included.
func okStatus(status int, body []byte) bool {
	if status >= 200 && status < 300 {
		return true
	}
	if status == 401 || status == 403 {
		writeError("unauthorized",
			"BlueBubbles rejected the password Aileron sent. "+
				"Run `aileron binding setup github://ALRubinger/aileron-connector-bluebubbles` "+
				"to re-enter the BlueBubbles server password.")
		return false
	}
	writeError("external_api_error",
		fmt.Sprintf("BlueBubbles returned HTTP %d: %s", status, tail(body, 512)))
	return false
}

func writeOutput(out map[string]any) {
	_ = json.NewEncoder(os.Stdout).Encode(output{Output: out})
}

func writeError(class, message string) {
	aileronLog("error", message)
	_ = json.NewEncoder(os.Stdout).Encode(output{Error: &outputError{Class: class, Message: message}})
}
