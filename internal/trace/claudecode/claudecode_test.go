package claudecode

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/trace"
)

func steps(t *testing.T, transcript string) []trace.Step {
	t.Helper()
	var out []trace.Step
	if _, err := Read(strings.NewReader(transcript), func(s trace.Step) { out = append(out, s) }); err != nil {
		t.Fatal(err)
	}
	return out
}

func kinds(ss []trace.Step) []trace.Kind {
	out := make([]trace.Kind, len(ss))
	for i, s := range ss {
		out[i] = s.Kind
	}
	return out
}

// TestRead_StepsInFileOrder: every record opens with an Entry that carries
// its stamp, session, cwd and sidechain flag, and its steps follow in order:
// a call and its text, then the result before the person's words.
func TestRead_StepsInFileOrder(t *testing.T) {
	got := steps(t, `{"type":"assistant","sessionId":"s1","timestamp":"2026-11-10T10:00:00Z","cwd":"/w","message":{"content":[{"type":"text","text":"Running."},{"type":"tool_use","id":"c1","name":"Bash","input":{"command":"go test ./..."}}]}}
{"type":"user","sessionId":"s1","isSidechain":true,"message":{"content":[{"type":"tool_result","tool_use_id":"c1","is_error":false,"content":"ok"},{"type":"text","text":"thanks"}]}}
`)
	want := []trace.Kind{trace.Entry, trace.Message, trace.Text, trace.Call, trace.Entry, trace.Result, trace.Said}
	if g := kinds(got); !equalKinds(g, want) {
		t.Fatalf("kinds %v, want %v", g, want)
	}
	if got[0].Session != "s1" || got[0].Cwd != "/w" || got[0].At.IsZero() || got[3].At != got[0].At {
		t.Errorf("entry fields not carried: %+v / %+v", got[0], got[3])
	}
	if c := got[3].Call; c.ID != "c1" || c.Tool != "Bash" || c.Class != trace.ActExec {
		t.Errorf("call %+v", c)
	}
	if !got[5].Sidechain || got[5].Result.CallID != "c1" || got[5].Result.Failed {
		t.Errorf("result %+v", got[5])
	}
}

func equalKinds(a, b []trace.Kind) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestRead_PassesOver: Claude Code's own context lines, other record types,
// torn records and odd contents yield no step but their Entry, or none.
func TestRead_PassesOver(t *testing.T) {
	got := steps(t, `{"type":"user","isMeta":true,"message":{"content":"caveat"}}
{"type":"progress","data":"x"}
{"type":"user","message":{"content":
{"type":"assistant","message":{"content":"a string"}}
{"type":"user","message":{"content":{"odd":true}}}
`)
	want := []trace.Kind{trace.Entry, trace.Entry, trace.Entry}
	if g := kinds(got); !equalKinds(g, want) {
		t.Fatalf("kinds %v, want %v", g, want)
	}
}

// TestSaid_OriginAndSignal: markup is the machine's whatever the record
// says; the origin mark says the person or a peer; with no mark the origin
// is unknown; the interrupt markers are signals.
func TestSaid_OriginAndSignal(t *testing.T) {
	for _, c := range []struct {
		record string
		origin trace.Origin
		signal trace.Signal
	}{
		{`{"type":"user","origin":{"kind":"human"},"message":{"content":"fix it"}}`, trace.OriginPerson, trace.SignalNone},
		{`{"type":"user","origin":{"kind":"peer"},"message":{"content":"from another session"}}`, trace.OriginPeer, trace.SignalNone},
		{`{"type":"user","message":{"content":"no mark"}}`, trace.OriginUnknown, trace.SignalNone},
		{`{"type":"user","origin":{"kind":"human"},"message":{"content":" <command-name>/clear</command-name>"}}`, trace.OriginMachine, trace.SignalNone},
		{`{"type":"user","message":{"content":[{"type":"text","text":"[Request interrupted by user]"}]}}`, trace.OriginUnknown, trace.SignalInterrupt},
		{`{"type":"user","message":{"content":[{"type":"text","text":"[Request interrupted by user for tool use]"}]}}`, trace.OriginUnknown, trace.SignalInterruptForTool},
	} {
		got := steps(t, c.record+"\n")
		s := got[len(got)-1]
		if s.Kind != trace.Said || s.Origin != c.origin || s.Signal != c.signal {
			t.Errorf("%s: kind %v origin %v signal %v, want said %v %v", c.record, s.Kind, s.Origin, s.Signal, c.origin, c.signal)
		}
	}
}

// TestResult_Denials: the denial kind decides when Claude Code writes it;
// without it the text does; a kind this adapter does not know is
// unrecognised; a tool error's opening names its kind; a command's own exit
// is no tool error.
func TestResult_Denials(t *testing.T) {
	const rejected = "The user doesn't want to proceed with this tool use. The tool use was rejected (eg. if it was a file edit, the new_string was NOT written to the file). STOP what you are doing and wait for the user to tell you how to proceed."
	rec := func(isError bool, content, kind string) string {
		k := ""
		if kind != "" {
			k = `,"toolDenialKind":"` + kind + `"`
		}
		e := "false"
		if isError {
			e = "true"
		}
		return `{"type":"user"` + k + `,"message":{"content":[{"type":"tool_result","tool_use_id":"c","is_error":` + e + `,"content":` + quote(content) + `}]}}` + "\n"
	}
	for _, c := range []struct {
		name   string
		record string
		failed bool
		denied trace.Denial
		err    trace.ToolError
	}{
		{"ok", rec(false, "fine", "user-rejected"), false, trace.DeniedNone, trace.ErrorNone},
		{"person by kind", rec(true, "anything", "user-rejected"), true, trace.DeniedByPerson, trace.ErrorNone},
		{"rule by kind", rec(true, "hook said no", "permission-rule"), true, trace.DeniedByRule, trace.ErrorNone},
		{"unknown kind", rec(true, rejected, "future-kind"), true, trace.DeniedUnrecognised, trace.ErrorNone},
		{"person by text", rec(true, rejected, ""), true, trace.DeniedByPerson, trace.ErrorNone},
		{"settings rule by text", rec(true, "Permission to use Bash with command rm has been denied.", ""), true, trace.DeniedOther, trace.ErrorNone},
		{"tool error", rec(true, "<tool_use_error>File does not exist.</tool_use_error>", ""), true, trace.DeniedNone, trace.ErrorNoSuchFile},
		{"command exit", rec(true, "Exit code 1\nFAIL", ""), true, trace.DeniedNone, trace.ErrorNone},
	} {
		got := steps(t, c.record)
		r := got[len(got)-1].Result
		if r == nil || r.Failed != c.failed || r.Denied != c.denied || r.Error != c.err {
			t.Errorf("%s: result %+v, want failed %v denied %v error %v", c.name, r, c.failed, c.denied, c.err)
		}
	}
}

func quote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return `"` + r.Replace(s) + `"`
}

// TestActClass_EveryBuiltInTool: each of Claude Code's tools has its class,
// an MCP tool is external, and a tool this adapter does not know is other.
func TestActClass_EveryBuiltInTool(t *testing.T) {
	for name, want := range map[string]trace.ActClass{
		"Bash": trace.ActExec, "BashOutput": trace.ActExecControl, "KillShell": trace.ActExecControl,
		"Edit": trace.ActEdit, "MultiEdit": trace.ActEdit, "NotebookEdit": trace.ActEdit, "Write": trace.ActWrite,
		"Read": trace.ActRead, "NotebookRead": trace.ActRead, "Glob": trace.ActSearch, "Grep": trace.ActSearch,
		"WebFetch": trace.ActFetch, "WebSearch": trace.ActFetch, "Task": trace.ActDelegate, "Agent": trace.ActDelegate,
		"ExitPlanMode": trace.ActPlan, "AskUserQuestion": trace.ActAsk,
		"mcp__srv__deploy": trace.ActExternal, "Frobnicate": trace.ActOther,
	} {
		if got := ActClass(name); got != want {
			t.Errorf("ActClass(%q) = %v, want %v", name, got, want)
		}
	}
}

// TestToolError_Openings: each opening Claude Code writes maps to its kind.
func TestToolError_Openings(t *testing.T) {
	for text, want := range map[string]trace.ToolError{
		"<tool_use_error>String to replace not found in file.":                                  trace.ErrorEditNotFound,
		"<tool_use_error>Found 3 matches of the string to replace, but replace_all is false.":   trace.ErrorEditAmbiguous,
		"<tool_use_error>No changes to make: old_string and new_string are exactly the same.":   trace.ErrorEditNoChange,
		"<tool_use_error>File has been modified since read, either by the user or by a linter.": trace.ErrorStaleWrite,
		"<tool_use_error>File has not been read yet. Read it first before writing to it.":       trace.ErrorUnreadWrite,
		"File content (300000 tokens) exceeds maximum allowed tokens (25000).":                  trace.ErrorTooLarge,
		"<tool_use_error>File does not exist.":                                                  trace.ErrorNoSuchFile,
		"InputValidationError: Edit failed due to the following issue":                          trace.ErrorBadInput,
		"Exit code 2":             trace.ErrorNone,
		"something else entirely": trace.ErrorNone,
	} {
		if got := ToolError(text); got != want {
			t.Errorf("ToolError(%q) = %v, want %v", text, got, want)
		}
	}
}
