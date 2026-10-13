package store

import (
	"reflect"
	"strings"
	"testing"
)

// jsonKeys lists a struct's json keys in field order, embedded structs
// flattened as encoding/json flattens them.
func jsonKeys(t reflect.Type) []string {
	var keys []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			keys = append(keys, jsonKeys(f.Type)...)
			continue
		}
		keys = append(keys, strings.Split(f.Tag.Get("json"), ",")[0])
	}
	return keys
}

// TestREC27b_SessionRecordHasOneClaudeCodeVersionField: the session record
// carries exactly one claude_code_version key, with its source beside it, in
// every spelling a reader could key on (REC-27).
func TestREC27b_SessionRecordHasOneClaudeCodeVersionField(t *testing.T) {
	n := 0
	for _, k := range jsonKeys(reflect.TypeOf(Session{})) {
		if strings.EqualFold(k, "claude_code_version") || strings.EqualFold(strings.ReplaceAll(k, "-", "_"), "claude_code_version") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("session record has %d claude_code_version keys, want exactly one", n)
	}
	keys := strings.Join(jsonKeys(reflect.TypeOf(Session{})), ",")
	if !strings.Contains(keys, ",claude_code_version,claude_code_version_source,") {
		t.Error("session record does not carry claude_code_version_source right after claude_code_version")
	}
	if _, ok := reflect.TypeOf(Session{}).MethodByName("MarshalJSON"); ok {
		t.Error("a custom MarshalJSON could emit keys this test cannot see")
	}
}

// header is every schema-5 kind's leading keys (1.2.2: critical, seq null on
// spill).
var header = []string{"type", "schema_version", "critical", "seq", "recorded_at_unix_ms", "session_id"}

func withHeader(keys ...string) []string { return append(append([]string{}, header...), keys...) }

// TestDEL22b_SchemaFiveKeyListsAreFrozen pins the schema-5 field list (1.2.3 to
// 1.2.10; archive 10.2.2 to 10.2.14) as literal key lists for every v5 type,
// nested types included, so a key added, renamed or reordered is a visible
// contract change. No omitempty anywhere: every v5 key is always present.
func TestDEL22b_SchemaFiveKeyListsAreFrozen(t *testing.T) {
	cases := []struct {
		typ  reflect.Type
		want []string
	}{
		{reflect.TypeOf(Cut{}), []string{"offset", "removed_bytes"}},
		{reflect.TypeOf(MaskSpan{}), []string{"rule", "start", "end"}},
		{reflect.TypeOf(DeclarationV5{}), []string{"writer", "command", "command_bytes", "command_cut", "masked", "command_withheld", "cwd", "project", "file_path", "written_paths", "written_paths_complete", "target", "edit_marks_before"}},
		{reflect.TypeOf(ExecutionV5{}), []string{"writer", "executed_command", "executed_command_reason", "executed_command_bytes", "executed_command_cut", "executed_masked", "failure", "output_evidence", "edit_marks", "head_after", "gate_output", "credmech"}},
		{reflect.TypeOf(TerminalV5{}), []string{"writer"}},
		{reflect.TypeOf(Target{}), []string{"aws_profile", "aws_region", "aws_account", "endpoint_host", "s3_bucket", "kube_context", "kube_namespace", "gcp_project", "azure_subscription", "source", "dropped"}},
		{reflect.TypeOf(Failure{}), []string{"origin", "class", "source", "counts_as_failure", "counts_for_learning", "class_table_version", "error_bytes"}},
		{reflect.TypeOf(OutputEvidence{}), []string{"family", "success_line", "fail_line", "scanned_bytes", "version"}},
		{reflect.TypeOf(EditMarks{}), []string{"added", "removed", "basis", "version"}},
		{reflect.TypeOf(GateOutput{}), []string{"kind", "parser", "reason"}},
		{reflect.TypeOf(GitState{}), []string{"root", "head", "branch", "detached", "state"}},
		{reflect.TypeOf(EventHeader{}), header},
		{reflect.TypeOf(TurnEnd{}), withHeader("prompt_id", "event", "stop_hook_active", "api_error", "model", "model_source", "claude_code_version", "effort", "permission_mode", "git", "background_tasks", "calls", "claim_class", "claim_words", "gate_state", "gate_version", "cost", "recap_spoke", "stood_down")},
		{reflect.TypeOf(BackgroundTasks{}), []string{"present", "tasks", "crons"}},
		{reflect.TypeOf(BackgroundTask{}), []string{"id", "type", "status", "runner", "command"}},
		{reflect.TypeOf(TurnCalls{}), []string{"declared", "executed", "failed", "interrupted", "main", "subagent"}},
		{reflect.TypeOf(TurnCostBody{}), []string{"state", "model_turns_main", "model_turns_subagents", "input_tokens", "output_tokens", "cache_read_tokens", "cache_write_tokens", "usd_nano", "unpriced_responses", "price_table", "models"}},
		{reflect.TypeOf(TurnCost{}), withHeader("prompt_id", "cost")},
		{reflect.TypeOf(Prompt{}), withHeader("prompt_id", "prompt_source", "prompt_class", "prompt_bytes", "previous_prompt_id", "previous_turn_state", "git", "permission_mode")},
		{reflect.TypeOf(Human{}), withHeader("prompt_id", "agent_id", "kind", "source", "tool_name", "tool_use_id", "joined_by", "reason", "evidence")},
		{reflect.TypeOf(RecordScopes{}), []string{"record_scope", "switches"}},
		{reflect.TypeOf(ResumeFields{}), []string{"seconds_since_last_response", "context_tokens"}},
		{reflect.TypeOf(Session{}), withHeader("source", "model", "agent_type", "harness", "harness_evidence", "writer", "rules_version", "class_table_version", "masker_version", "project", "git", "scopes", "entries", "steer_state", "claude_code_version", "claude_code_version_source", "could_not_see", "resume")},
		{reflect.TypeOf(Subagent{}), withHeader("prompt_id", "agent_id", "agent_type", "phase", "internal", "claim_class", "stop_hook_active", "background_present")},
		{reflect.TypeOf(Batch{}), withHeader("prompt_id", "tool_use_ids", "size")},
		{reflect.TypeOf(Task{}), withHeader("prompt_id", "event", "task_id", "subject", "agent_id", "teammate_name")},
		{reflect.TypeOf(Compact{}), withHeader("prompt_id", "trigger", "summary_claim_class", "summary_bytes")},
		{reflect.TypeOf(Instructions{}), withHeader("prompt_id", "agent_id", "file_path", "memory_type", "load_reason", "trigger_file_path", "globs_count", "file_sha256")},
		{reflect.TypeOf(Model{}), withHeader("prompt_id", "from", "to", "requested", "source")},
		{reflect.TypeOf(OutcomeJoin{}), withHeader("tool_use_id", "outcome", "exit_code", "outcome_source", "parser_version")},
		{reflect.TypeOf(WriteObserved{}), withHeader("path", "edit_count")},
	}
	seen := map[reflect.Type]bool{}
	for _, c := range cases {
		seen[c.typ] = true
		if got := jsonKeys(c.typ); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s keys = %v, want %v", c.typ.Name(), got, c.want)
		}
		if _, ok := c.typ.MethodByName("MarshalJSON"); ok {
			t.Errorf("%s has a custom MarshalJSON; the key list would not be what the test reads", c.typ.Name())
		}
		for i := 0; i < c.typ.NumField(); i++ {
			if strings.Contains(c.typ.Field(i).Tag.Get("json"), "omitempty") {
				t.Errorf("%s.%s is omitempty; every v5 key is always present", c.typ.Name(), c.typ.Field(i).Name)
			}
		}
	}
	// The fields the archive types beyond the key: a map or a list that
	// became a scalar would keep its key and lose its meaning.
	for _, c := range []struct {
		typ   reflect.Type
		field string
		want  string
	}{
		{reflect.TypeOf(Target{}), "Source", "map[string]string"},
		{reflect.TypeOf(Target{}), "Dropped", "[]string"},
		{reflect.TypeOf(EditMarks{}), "Added", "map[string]int"},
		{reflect.TypeOf(EditMarks{}), "Removed", "map[string]int"},
		{reflect.TypeOf(DeclarationV5{}), "EditMarksBefore", "map[string]int"},
		{reflect.TypeOf(BackgroundTasks{}), "Tasks", "[]store.BackgroundTask"},
		{reflect.TypeOf(Session{}), "Scopes", "store.RecordScopes"},
		{reflect.TypeOf(Session{}), "Resume", "*store.ResumeFields"},
		{reflect.TypeOf(EventHeader{}), "Seq", "*int64"},
		{reflect.TypeOf(EventHeader{}), "Critical", "bool"},
	} {
		f, ok := c.typ.FieldByName(c.field)
		if !ok {
			t.Errorf("%s has no field %s", c.typ.Name(), c.field)
			continue
		}
		if f.Type.String() != c.want {
			t.Errorf("%s.%s is %s, want %s", c.typ.Name(), c.field, f.Type, c.want)
		}
	}
	// Every struct a v5 type reaches is itself pinned above, so a nested
	// shape cannot change unnoticed.
	for _, c := range cases {
		for i := 0; i < c.typ.NumField(); i++ {
			ft := c.typ.Field(i).Type
			for ft.Kind() == reflect.Pointer || ft.Kind() == reflect.Slice || ft.Kind() == reflect.Map {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct && !seen[ft] {
				t.Errorf("%s.%s reaches %s, which has no key list here", c.typ.Name(), c.typ.Field(i).Name, ft.Name())
			}
		}
	}
	// Every new kind of kinds.go has a struct whose header type matches.
	kinds := map[string]reflect.Type{
		TypeSession: reflect.TypeOf(Session{}), TypePrompt: reflect.TypeOf(Prompt{}), TypeTurnEnd: reflect.TypeOf(TurnEnd{}),
		TypeTurnCost: reflect.TypeOf(TurnCost{}), TypeHuman: reflect.TypeOf(Human{}), TypeSubagent: reflect.TypeOf(Subagent{}),
		TypeBatch: reflect.TypeOf(Batch{}), TypeTask: reflect.TypeOf(Task{}), TypeCompact: reflect.TypeOf(Compact{}),
		TypeInstructions: reflect.TypeOf(Instructions{}), TypeModel: reflect.TypeOf(Model{}),
		TypeOutcomeJoin: reflect.TypeOf(OutcomeJoin{}), TypeWriteObserved: reflect.TypeOf(WriteObserved{}),
	}
	for kind := range KindVersion {
		switch kind {
		case TypeDeclaration, TypeExecution, TypeTerminal, TypeCoverage, TypeGap, TypeVerify:
			continue // call records extend the v4 structs; the verify checkpoint lives in internal/verify
		}
		typ, ok := kinds[kind]
		if !ok {
			t.Errorf("kind %s has no schema-5 struct", kind)
			continue
		}
		if got := jsonKeys(typ)[:len(header)]; !reflect.DeepEqual(got, header) {
			t.Errorf("kind %s does not start with the schema-5 header: %v", kind, got)
		}
	}
}
