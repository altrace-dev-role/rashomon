package hook

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/hookio"
)

// The structs every hook entry decoded its payload into before the payload
// was scanned, as they stood, less scratchpad_dir, which was never read: the
// oracle FuzzRES5_IntakeMatchesTheStructDecode holds the scan to.
type (
	oldPayload struct {
		SessionID      string          `json:"session_id"`
		PromptID       string          `json:"prompt_id"`
		TranscriptPath string          `json:"transcript_path"`
		CWD            string          `json:"cwd"`
		PermissionMode string          `json:"permission_mode"`
		HookEventName  string          `json:"hook_event_name"`
		ToolName       string          `json:"tool_name"`
		ToolInput      json.RawMessage `json:"tool_input"`
		ToolUseID      string          `json:"tool_use_id"`
		AgentID        string          `json:"agent_id"`
		AgentType      string          `json:"agent_type"`
	}
	oldPostPayload struct {
		SessionID     string          `json:"session_id"`
		ToolName      string          `json:"tool_name"`
		ToolUseID     string          `json:"tool_use_id"`
		HookEventName string          `json:"hook_event_name"`
		Error         string          `json:"error"`
		IsInterrupt   *bool           `json:"is_interrupt"`
		DurationMS    *int64          `json:"duration_ms"`
		ToolInput     json.RawMessage `json:"tool_input"`
		CWD           string          `json:"cwd"`
	}
	oldSessionPayload struct {
		SessionID string `json:"session_id"`
		CWD       string `json:"cwd"`
	}
	oldBackgroundResponse struct {
		ToolResponse struct {
			BackgroundTaskID   oldPresent `json:"backgroundTaskId"`
			BackgroundedByUser oldPresent `json:"backgroundedByUser"`
		} `json:"tool_response"`
	}
)

type oldPresent bool

func (p *oldPresent) UnmarshalJSON(b []byte) error {
	v := bytes.TrimSpace(b)
	*p = oldPresent(!bytes.Equal(v, []byte("null")) && !bytes.Equal(v, []byte("false")))
	return nil
}

var structKeys = []string{
	"session_id", "prompt_id", "transcript_path", "cwd", "permission_mode", "hook_event_name",
	"tool_name", "tool_input", "tool_use_id", "agent_id", "agent_type", "error", "is_interrupt",
	"duration_ms", "tool_response",
}

// repeats reports an object in which some key of want occurs twice, case
// folded as a struct field matches it. The scan reads every key from its
// last copy; the struct decode kept an earlier string under a later null,
// failed on any copy of the wrong type and merged repeated objects, and
// payloads Claude Code writes repeat no key, so these inputs are not held
// to the struct decode.
func repeats(raw []byte, want []string) bool {
	d := json.NewDecoder(bytes.NewReader(raw))
	if t, err := d.Token(); err != nil || t != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	for d.More() {
		t, err := d.Token()
		if err != nil {
			return false
		}
		key, _ := t.(string)
		for _, w := range want {
			if strings.EqualFold(key, w) {
				if seen[w] {
					return true
				}
				seen[w] = true
			}
		}
		var v json.RawMessage
		if d.Decode(&v) != nil {
			return false
		}
		if strings.EqualFold(key, "tool_response") && repeats(v, []string{"backgroundTaskId", "backgroundedByUser"}) {
			return true
		}
	}
	return false
}

// FuzzRES5_IntakeMatchesTheStructDecode: every field an entry fills from
// the scan is what decoding into the old struct gave, keys matched in any
// case, and the scan fails exactly where the struct decode failed. Break:
// match payload keys exactly, and a case-variant key is lost.
func FuzzRES5_IntakeMatchesTheStructDecode(f *testing.F) {
	for _, s := range []string{
		`{"session_id":"s","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_use_id":"t","cwd":"/p"}`,
		`{"Last_Assistant_Message":"hi","SESSION_ID":"s","Tool_Name":"Bash","CWD":"/c"}`,
		`{"session_id":"s","hook_event_name":"PostToolUseFailure","error":"Exit code 2\nboom","is_interrupt":false,"duration_ms":12}`,
		`{"session_id":"s","tool_name":"Bash","tool_response":{"backgroundTaskId":"b1","stdout":""}}`,
		`{"TOOL_RESPONSE":{"BackgroundedByUser":true}}`,
		`{"session_id":5}`,
		`{"duration_ms":1.5}`,
		`{"is_interrupt":"yes"}`,
		`{"error":["x"]}`,
		`{"tool_input":null,"agent_id":null}`,
		`{"ſession_id":"long s","tool_use_ıd":"dotless i"}`,
		`{"tool_response":"not an object"}`,
		`null`,
		`[]`,
		`"{\"session_id\":\"s\"}"`,
		`{"session_id":"\ud800","cwd":"\u00e9"}`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > hookio.MaxPayloadBytes || repeats(raw, structKeys) {
			return
		}
		in, ierr := hookio.Intake(bytes.NewReader(raw))

		var op oldPayload
		oerr := json.Unmarshal(raw, &op)
		var np Payload
		nerr := ierr
		if nerr == nil {
			nerr = np.from(in)
		}
		if (oerr == nil) != (nerr == nil) {
			t.Fatalf("Payload: struct decode %v, scan %v, on %q", oerr, nerr, raw)
		}
		if oerr == nil {
			want := Payload{SessionID: op.SessionID, PromptID: op.PromptID, TranscriptPath: op.TranscriptPath,
				CWD: op.CWD, PermissionMode: op.PermissionMode, HookEventName: op.HookEventName, ToolName: op.ToolName,
				ToolInput: op.ToolInput, ToolUseID: op.ToolUseID, AgentID: op.AgentID, AgentType: op.AgentType}
			if !samePayload(np, want) {
				t.Fatalf("Payload on %q:\n scan %+v\nwant %+v", raw, np, want)
			}
		}

		var os oldSessionPayload
		oerr = json.Unmarshal(raw, &os)
		var ns sessionPayload
		nerr = ierr
		if nerr == nil {
			nerr = ns.from(in)
		}
		if (oerr == nil) != (nerr == nil) || (oerr == nil && (ns.SessionID != os.SessionID || ns.CWD != os.CWD)) {
			t.Fatalf("sessionPayload on %q: scan %+v %v, struct %+v %v", raw, ns, nerr, os, oerr)
		}

		var opp oldPostPayload
		oerr = json.Unmarshal(raw, &opp)
		var npp PostPayload
		nerr = ierr
		if nerr == nil {
			nerr = npp.from(in)
		}
		if (oerr == nil) != (nerr == nil) {
			t.Fatalf("PostPayload: struct decode %v, scan %v, on %q", oerr, nerr, raw)
		}
		if oerr == nil {
			if npp.SessionID != opp.SessionID || npp.ToolName != opp.ToolName || npp.ToolUseID != opp.ToolUseID ||
				npp.HookEventName != opp.HookEventName || npp.CWD != opp.CWD || !bytes.Equal(npp.ToolInput, opp.ToolInput) ||
				!sameBool(npp.IsInterrupt, opp.IsInterrupt) || !sameInt(npp.DurationMS, opp.DurationMS) ||
				!strings.HasPrefix(opp.Error, npp.Error) || (len(opp.Error) <= errorHeadBytes && npp.Error != opp.Error) {
				t.Fatalf("PostPayload on %q:\n scan %+v\nwant %+v", raw, npp, opp)
			}
		}

		if ierr == nil {
			var ob oldBackgroundResponse
			wantBg := json.Unmarshal(raw, &ob) == nil && bool(ob.ToolResponse.BackgroundTaskID || ob.ToolResponse.BackgroundedByUser)
			if got := backgroundedCall(in); got != wantBg {
				t.Fatalf("backgroundedCall on %q = %v, struct decode %v", raw, got, wantBg)
			}
		}
	})
}

func samePayload(a, b Payload) bool {
	return a.SessionID == b.SessionID && a.PromptID == b.PromptID && a.TranscriptPath == b.TranscriptPath &&
		a.CWD == b.CWD && a.PermissionMode == b.PermissionMode && a.HookEventName == b.HookEventName &&
		a.ToolName == b.ToolName && a.ToolUseID == b.ToolUseID && a.AgentID == b.AgentID &&
		a.AgentType == b.AgentType && bytes.Equal(a.ToolInput, b.ToolInput)
}

func sameBool(a, b *bool) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }

func sameInt(a, b *int64) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }

// TestRES5_BackgroundKeysMatchTheirOldFields: the two keys backgroundedCall
// reads are found in any case, as the struct fields that decoded them were.
func TestRES5_BackgroundKeysMatchTheirOldFields(t *testing.T) {
	in, err := hookio.Intake(strings.NewReader(`{"Tool_Response":{"BACKGROUNDTASKID":"b1"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !backgroundedCall(in) {
		t.Error("a case-variant backgroundTaskId was not read")
	}
}
