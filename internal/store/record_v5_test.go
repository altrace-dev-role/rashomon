package store

import (
	"bytes"
	"encoding"
	"encoding/json"
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
	if _, ok := reflect.PointerTo(reflect.TypeOf(Session{})).MethodByName("MarshalJSON"); ok {
		t.Error("a custom MarshalJSON could emit keys this test cannot see")
	}
}

// header is every schema-5 kind's leading keys and their types (1.2.2:
// critical, seq null on spill).
var header = []string{"type string", "schema_version int", "critical bool", "seq *int64", "recorded_at_unix_ms int64", "session_id string"}

func withHeader(keys ...string) []string { return append(append([]string{}, header...), keys...) }

// v5Lists is the schema-5 field list (1.2.3 to 1.2.11; archive 10.2.2 to
// 10.2.14, 10.2.17; BREADTH's keys, tech 6.2.1) as literal "key type" pairs
// for every v5 type, nested types included, in field order. A key added,
// renamed, reordered or retyped is a visible contract change here.
var v5Lists = []struct {
	typ  reflect.Type
	want []string
}{
	{reflect.TypeOf(Cut{}), []string{"offset int", "removed_bytes int"}},
	{reflect.TypeOf(MaskSpan{}), []string{"rule string", "start int", "end int"}},
	{reflect.TypeOf(DeclarationV5{}), []string{
		"writer string", "command *string", "command_bytes *int", "command_cut *store.Cut", "masked []store.MaskSpan",
		"command_withheld *string", "cwd *string", "project *string", "file_path *string", "written_paths []string",
		"written_paths_complete *bool", "target *store.Target", "edit_marks_before map[string]int",
		"written_paths_withheld *string", "file_path_withheld *string", "cwd_withheld *string", "project_withheld *string",
		"target_withheld *string", "edit_marks_before_withheld *string",
		"harness_id *string", "harness_tool_name *string", "declared_via *string",
	}},
	{reflect.TypeOf(ExecutionV5{}), []string{
		"writer string", "executed_command *string", "executed_command_reason *string", "executed_command_bytes *int",
		"executed_command_cut *store.Cut", "executed_masked []store.MaskSpan", "failure *store.Failure",
		"output_evidence *store.OutputEvidence", "edit_marks *store.EditMarks", "head_after *store.GitState",
		"gate_output *store.GateOutput", "credmech *string",
		"executed_command_withheld *string", "failure_withheld *string", "output_evidence_withheld *string",
		"edit_marks_withheld *string", "head_after_withheld *string",
		"harness_id *string", "harness_tool_name *string", "declared_via *string",
		"outcome_source *string", "failure_type *string", "mcp_server_source *string",
	}},
	{reflect.TypeOf(TerminalV5{}), []string{"writer string"}},
	{reflect.TypeOf(Target{}), []string{
		"aws_profile *string", "aws_region *string", "aws_account *string", "endpoint_host *string", "s3_bucket *string",
		"kube_context *string", "kube_namespace *string", "gcp_project *string", "azure_subscription *string",
		"source map[string]string", "dropped []string",
	}},
	{reflect.TypeOf(Failure{}), []string{"origin string", "class string", "source string", "counts_as_failure bool", "counts_for_learning bool", "class_table_version int", "error_bytes int"}},
	{reflect.TypeOf(OutputEvidence{}), []string{"family string", "success_line bool", "fail_line bool", "scanned_bytes int", "version int"}},
	{reflect.TypeOf(EditMarks{}), []string{"added map[string]int", "removed map[string]int", "basis string", "version int"}},
	{reflect.TypeOf(GateOutput{}), []string{"kind string", "parser string", "reason *string"}},
	{reflect.TypeOf(GitState{}), []string{"root *string", "head *string", "branch *string", "detached bool", "state string"}},
	{reflect.TypeOf(EventHeader{}), header},
	{reflect.TypeOf(TurnEnd{}), withHeader(
		"prompt_id *string", "event string", "stop_hook_active *bool", "api_error *string", "model *string",
		"model_source string", "claude_code_version *string", "effort *string", "permission_mode *string",
		"git *store.GitState", "background_tasks store.BackgroundTasks", "calls store.TurnCalls", "claim_class string",
		"claim_words []string", "gate_state *string", "gate_version *int", "cost *store.TurnCostBody",
		"recap_spoke bool", "stood_down []string")},
	{reflect.TypeOf(BackgroundTasks{}), []string{"present bool", "tasks []store.BackgroundTask", "crons *int"}},
	{reflect.TypeOf(BackgroundTask{}), []string{"id string", "type string", "status string", "runner bool", "command *string"}},
	{reflect.TypeOf(TurnCalls{}), []string{"declared int", "executed int", "failed int", "interrupted int", "main int", "subagent int"}},
	{reflect.TypeOf(TurnCostBody{}), []string{
		"state string", "model_turns_main int", "model_turns_subagents int", "input_tokens int64", "output_tokens int64",
		"cache_read_tokens int64", "cache_write_tokens int64", "usd_nano int64", "unpriced_responses int",
		"price_table string", "models []string",
	}},
	{reflect.TypeOf(TurnCost{}), withHeader("prompt_id *string", "cost store.TurnCostBody")},
	{reflect.TypeOf(Prompt{}), withHeader(
		"prompt_id *string", "prompt_source string", "prompt_class *string", "prompt_bytes int",
		"previous_prompt_id *string", "previous_turn_state string", "git *store.GitState", "permission_mode *string")},
	{reflect.TypeOf(Human{}), withHeader(
		"prompt_id *string", "agent_id *string", "kind string", "source string", "tool_name *string",
		"tool_use_id *string", "joined_by string", "reason *string", "evidence []string")},
	{reflect.TypeOf(RecordScopes{}), []string{"record_scope string", "switches map[string]bool"}},
	{reflect.TypeOf(ResumeFields{}), []string{"seconds_since_last_response *int64", "context_tokens *int64"}},
	{reflect.TypeOf(Session{}), withHeader(
		"source string", "model *string", "agent_type *string", "harness string", "harness_evidence []string",
		"writer string", "rules_version int", "class_table_version int", "masker_version int", "project *string",
		"git *store.GitState", "scopes store.RecordScopes", "entries map[string]string", "steer_state string",
		"claude_code_version *string", "claude_code_version_source string", "could_not_see []string",
		"resume *store.ResumeFields")},
	{reflect.TypeOf(Subagent{}), withHeader(
		"prompt_id *string", "agent_id *string", "agent_type *string", "phase string", "internal bool",
		"claim_class *string", "stop_hook_active *bool", "background_present *bool")},
	{reflect.TypeOf(Batch{}), withHeader("prompt_id *string", "tool_use_ids []string", "size int")},
	{reflect.TypeOf(Task{}), withHeader("prompt_id *string", "event string", "task_id string", "subject *string", "agent_id *string", "teammate_name *string")},
	{reflect.TypeOf(Compact{}), withHeader("prompt_id *string", "trigger string", "summary_claim_class string", "summary_bytes int")},
	{reflect.TypeOf(Instructions{}), withHeader(
		"prompt_id *string", "agent_id *string", "file_path string", "memory_type string", "load_reason string",
		"trigger_file_path *string", "globs_count *int", "file_sha256 *string")},
	{reflect.TypeOf(Model{}), withHeader("prompt_id *string", "from *string", "to string", "requested *string", "source string")},
	{reflect.TypeOf(OutcomeJoin{}), withHeader("tool_use_id string", "outcome string", "exit_code *int", "outcome_source string", "parser_version int")},
	{reflect.TypeOf(WriteObserved{}), withHeader("path string", "edit_count int")},
}

// TestDEL22b_SchemaFiveKeyListsAreFrozen pins the schema-5 field list in
// four ways, so no route changes the wire while the list stays green. The
// tags: every key is its tag exactly, with no option (omitempty, omitzero,
// string), and every embedded struct is untagged, so encoding/json
// flattens it as the list does. The list: key and type for every field,
// every struct a v5 type reaches pinned too. No marshaler: neither a type
// nor its pointer, nor anything promoted into either, implements
// json.Marshaler or encoding.TextMarshaler. The wire: a value with every
// leaf set is marshalled through a pointer and its keys and JSON kinds are
// compared with the list at every depth, and the zero value keeps every
// key (1.2.3: "All v5 keys always present").
func TestDEL22b_SchemaFiveKeyListsAreFrozen(t *testing.T) {
	pinned := map[reflect.Type][]reflect.StructField{}
	for _, c := range v5Lists {
		fields := tagCheckedFields(t, c.typ)
		pinned[c.typ] = fields
		var got []string
		for _, f := range fields {
			got = append(got, f.Tag.Get("json")+" "+f.Type.String())
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s keys = %q, want %q", c.typ.Name(), got, c.want)
		}
	}
	for typ, fields := range pinned {
		checkNoMarshaler(t, typ)
		for _, f := range fields {
			ft := f.Type
			for ft.Kind() == reflect.Pointer || ft.Kind() == reflect.Slice || ft.Kind() == reflect.Map {
				checkNoMarshaler(t, ft)
				if ft.Kind() == reflect.Map {
					checkNoMarshaler(t, ft.Key())
				}
				ft = ft.Elem()
			}
			checkNoMarshaler(t, ft)
			if ft.Kind() == reflect.Struct && pinned[ft] == nil {
				t.Errorf("%s.%s reaches %s, which has no key list here", typ.Name(), f.Name, ft.Name())
			}
		}
	}
	for _, c := range v5Lists {
		full := reflect.New(c.typ)
		fill(full.Elem())
		compareWire(t, c.typ.Name(), marshalDecode(t, full.Interface()), c.typ, pinned)
		zero := marshalDecode(t, reflect.New(c.typ).Interface()).(map[string]any)
		for _, f := range pinned[c.typ] {
			if _, ok := zero[f.Tag.Get("json")]; !ok {
				t.Errorf("the zero %s drops the key %q; every v5 key is always present", c.typ.Name(), f.Tag.Get("json"))
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
		if got := jsonKeys(typ)[:len(header)]; !reflect.DeepEqual(got, headerKeys()) {
			t.Errorf("kind %s does not start with the schema-5 header: %v", kind, got)
		}
	}
}

// headerKeys is header's keys without their types.
func headerKeys() []string {
	keys := make([]string, len(header))
	for i, h := range header {
		keys[i] = strings.Fields(h)[0]
	}
	return keys
}

// tagCheckedFields returns typ's fields as encoding/json renders them,
// embedded structs flattened, and fails on a tag the literal list cannot
// stand for: an option after the key, a "-" or an empty key, an unexported
// field, or a tagged embedded struct, which encoding/json nests.
func tagCheckedFields(t *testing.T, typ reflect.Type) []reflect.StructField {
	t.Helper()
	var out []reflect.StructField
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		tag := f.Tag.Get("json")
		switch {
		case f.Anonymous && tag == "" && f.Type.Kind() == reflect.Struct:
			out = append(out, tagCheckedFields(t, f.Type)...)
			continue
		case f.Anonymous:
			t.Errorf("%s embeds %s with the tag %q; encoding/json nests a tagged embed", typ.Name(), f.Type, tag)
		case !f.IsExported():
			t.Errorf("%s.%s is unexported and never marshalled", typ.Name(), f.Name)
		case tag == "" || tag == "-" || strings.Contains(tag, ","):
			t.Errorf("%s.%s has the tag %q; a v5 key is its tag exactly, with no option", typ.Name(), f.Name, tag)
		}
		out = append(out, f)
	}
	return out
}

var (
	jsonMarshaler = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	textMarshaler = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
)

// checkNoMarshaler fails when typ or *typ, whose method set holds every
// value-receiver, pointer-receiver and promoted method, can marshal itself.
func checkNoMarshaler(t *testing.T, typ reflect.Type) {
	t.Helper()
	for _, m := range []reflect.Type{jsonMarshaler, textMarshaler} {
		if typ.Implements(m) || typ.Kind() != reflect.Pointer && reflect.PointerTo(typ).Implements(m) {
			t.Errorf("%s implements %s; the list would not be what is written", typ, m)
		}
	}
}

// fill sets every leaf under v: a pointer to a filled value, one element
// in every slice and map, "x", 1 and true.
func fill(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		fill(v.Elem())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				fill(v.Field(i))
			}
		}
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 1, 1)
		fill(s.Index(0))
		v.Set(s)
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		k, e := reflect.New(v.Type().Key()).Elem(), reflect.New(v.Type().Elem()).Elem()
		fill(k)
		fill(e)
		m.SetMapIndex(k, e)
		v.Set(m)
	case reflect.String:
		v.SetString("x")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int64:
		v.SetInt(1)
	}
}

// marshalDecode marshals v and decodes the bytes generically, numbers kept
// as json.Number.
func marshalDecode(t *testing.T, v any) any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// compareWire checks a decoded value against the Go type that wrote it:
// an object with exactly the pinned keys for a struct, an array for a
// slice, an object for a map, and a string, number or boolean for a leaf.
func compareWire(t *testing.T, path string, got any, typ reflect.Type, pinned map[reflect.Type][]reflect.StructField) {
	t.Helper()
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	ok := true
	switch typ.Kind() {
	case reflect.Struct:
		obj, isObj := got.(map[string]any)
		if ok = isObj && len(obj) == len(pinned[typ]); ok {
			for _, f := range pinned[typ] {
				v, has := obj[f.Tag.Get("json")]
				if !has {
					t.Errorf("%s: the key %q is missing from the wire", path, f.Tag.Get("json"))
					continue
				}
				compareWire(t, path+"."+f.Tag.Get("json"), v, f.Type, pinned)
			}
		}
	case reflect.Slice:
		arr, isArr := got.([]any)
		if ok = isArr && len(arr) == 1; ok {
			compareWire(t, path+"[]", arr[0], typ.Elem(), pinned)
		}
	case reflect.Map:
		obj, isObj := got.(map[string]any)
		if ok = isObj && len(obj) == 1; ok {
			compareWire(t, path+"{}", obj["x"], typ.Elem(), pinned)
		}
	case reflect.String:
		_, ok = got.(string)
	case reflect.Bool:
		_, ok = got.(bool)
	case reflect.Int, reflect.Int64:
		_, ok = got.(json.Number)
	default:
		ok = false
	}
	if !ok {
		t.Errorf("%s is %T %v on the wire, which a %s does not write", path, got, got, typ)
	}
}
