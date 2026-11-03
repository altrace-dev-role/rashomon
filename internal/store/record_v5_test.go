package store

import (
	"bytes"
	"encoding"
	"encoding/json"
	"reflect"
	"slices"
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
		"prompt_id_withheld *string", "agent_id_withheld *string", "agent_type_withheld *string", "hosts_withheld *string", "ssh_hosts_withheld *string",
		"harness_id *string", "harness_tool_name *string", "declared_via *string",
	}},
	{reflect.TypeOf(ExecutionV5{}), []string{
		"writer string", "executed_command *string", "executed_command_reason *string", "executed_command_bytes *int",
		"executed_command_cut *store.Cut", "executed_masked []store.MaskSpan", "failure *store.Failure",
		"output_evidence *store.OutputEvidence", "edit_marks *store.EditMarks", "head_after *store.GitState",
		"gate_output *store.GateOutput", "credmech *string", "content_sha256 *string",
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
	{reflect.TypeOf(GitState{}), []string{"root *string", "head *string", "branch *string", "detached bool", "state string", "root_withheld *string"}},
	{reflect.TypeOf(EventHeader{}), header},
	{reflect.TypeOf(TurnEnd{}), withHeader(
		"prompt_id *string", "event string", "stop_hook_active *bool", "api_error *string", "model *string",
		"model_source string", "claude_code_version *string", "effort *string", "permission_mode *string",
		"git *store.GitState", "background_tasks store.BackgroundTasks", "calls store.TurnCalls", "claim_class string",
		"claim_words []string", "gate_state *string", "gate_version *int", "cost *store.TurnCostBody",
		"recap_spoke bool", "stood_down []string",
		"prompt_id_withheld *string", "permission_mode_withheld *string", "git_withheld *string", "cost_withheld *string")},
	{reflect.TypeOf(BackgroundTasks{}), []string{"present bool", "tasks []store.BackgroundTask", "crons *int"}},
	{reflect.TypeOf(BackgroundTask{}), []string{"id string", "type string", "status string", "runner bool", "command *string", "command_withheld *string"}},
	{reflect.TypeOf(TurnCalls{}), []string{"declared int", "executed int", "failed int", "interrupted int", "main int", "subagent int"}},
	{reflect.TypeOf(TurnCostBody{}), []string{
		"state string", "model_turns_main int", "model_turns_subagents int", "input_tokens int64", "output_tokens int64",
		"cache_read_tokens int64", "cache_write_tokens int64", "usd_nano int64", "unpriced_responses int",
		"price_table string", "models []string",
	}},
	{reflect.TypeOf(TurnCost{}), withHeader("prompt_id *string", "cost store.TurnCostBody", "prompt_id_withheld *string")},
	{reflect.TypeOf(Prompt{}), withHeader(
		"prompt_id *string", "prompt_source string", "prompt_class *string", "prompt_bytes int",
		"previous_prompt_id *string", "previous_turn_state string", "git *store.GitState", "permission_mode *string",
		"prompt_id_withheld *string", "previous_prompt_id_withheld *string", "prompt_class_withheld *string", "git_withheld *string", "permission_mode_withheld *string")},
	{reflect.TypeOf(Human{}), withHeader(
		"prompt_id *string", "agent_id *string", "kind string", "source string", "tool_name *string",
		"tool_use_id *string", "joined_by string", "reason *string", "evidence []string",
		"prompt_id_withheld *string", "agent_id_withheld *string", "tool_name_withheld *string", "tool_use_id_withheld *string")},
	{reflect.TypeOf(RecordScopes{}), []string{"record_scope string", "switches map[string]bool"}},
	{reflect.TypeOf(ResumeFields{}), []string{"seconds_since_last_response *int64", "context_tokens *int64"}},
	{reflect.TypeOf(Session{}), withHeader(
		"source string", "model *string", "agent_type *string", "harness string", "harness_evidence []string",
		"writer string", "rules_version int", "class_table_version int", "masker_version int", "project *string",
		"git *store.GitState", "scopes store.RecordScopes", "entries map[string]string", "steer_state string",
		"claude_code_version *string", "claude_code_version_source string", "could_not_see []string",
		"resume *store.ResumeFields", "agent_type_withheld *string", "project_withheld *string", "git_withheld *string")},
	{reflect.TypeOf(Subagent{}), withHeader(
		"prompt_id *string", "agent_id *string", "agent_type *string", "phase string", "internal bool",
		"claim_class *string", "stop_hook_active *bool", "background_present *bool",
		"prompt_id_withheld *string", "agent_id_withheld *string", "agent_type_withheld *string")},
	{reflect.TypeOf(Batch{}), withHeader("prompt_id *string", "tool_use_ids []string", "size int", "prompt_id_withheld *string")},
	{reflect.TypeOf(Task{}), withHeader("prompt_id *string", "event string", "task_id string", "subject *string", "agent_id *string", "teammate_name *string",
		"prompt_id_withheld *string", "subject_withheld *string", "agent_id_withheld *string")},
	{reflect.TypeOf(Compact{}), withHeader("prompt_id *string", "trigger string", "summary_claim_class string", "summary_bytes int", "prompt_id_withheld *string")},
	{reflect.TypeOf(Instructions{}), withHeader(
		"prompt_id *string", "agent_id *string", "file_path *string", "memory_type string", "load_reason string",
		"trigger_file_path *string", "globs_count *int", "file_sha256 *string",
		"prompt_id_withheld *string", "agent_id_withheld *string", "file_path_withheld *string", "trigger_file_path_withheld *string")},
	{reflect.TypeOf(Model{}), withHeader("prompt_id *string", "from *string", "to string", "requested *string", "source string", "prompt_id_withheld *string")},
	{reflect.TypeOf(OutcomeJoin{}), withHeader("tool_use_id string", "outcome string", "exit_code *int", "outcome_source string", "parser_version int")},
	{reflect.TypeOf(WriteObserved{}), withHeader("path *string", "edit_count int", "path_withheld *string")},
	{reflect.TypeOf(SpecEvent{}), withHeader("reason string", "event string", "rule_id string", "source_digest string", "landed_seq int64")},
	{reflect.TypeOf(AfterHit{}), withHeader("event string", "rule_id string", "source_digest string", "landed_seq int64",
		"exit_code *int", "foreground bool", "cwd_digest string", "maybe bool")},
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
		TypeSpecEvent: reflect.TypeOf(SpecEvent{}), TypeAfterHit: reflect.TypeOf(AfterHit{}),
	}
	for kind := range kindVersion {
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

// cappedKeys are the keys 1.2.11's cap table names (archive 10.2.15), the
// id spellings a v5 kind repeats them under (previous_prompt_id,
// trigger_file_path) and git.root; session_id is not one, since it buckets
// under unattributed instead (1.2.11).
var cappedKeys = []string{
	"tool_use_id", "prompt_id", "previous_prompt_id", "agent_id", "agent_type", "permission_mode", "tool_name",
	"file_path", "trigger_file_path", "cwd", "project", "transcript_path", "root", "path",
}

// withheldKeys is every field 1.2.11's caps or a record.* switch (1.2.12;
// archive 10.2.17) can null, by the v5 type that carries its
// <field>_withheld key: a capped key wherever a v5 kind has it, the v4 call
// record's nullable capped fields (prompt_id, agent_id, agent_type, hosts,
// ssh_hosts) on DeclarationV5, the switched fields (commands, paths,
// targets, edit_marks, failure_classes, output_evidence, git, cost,
// prompt_class, task_subjects), and background_tasks' commands.
var withheldKeys = map[reflect.Type][]string{
	reflect.TypeOf(DeclarationV5{}):  {"command", "cwd", "project", "file_path", "written_paths", "target", "edit_marks_before", "prompt_id", "agent_id", "agent_type", "hosts", "ssh_hosts"},
	reflect.TypeOf(ExecutionV5{}):    {"executed_command", "failure", "output_evidence", "edit_marks", "head_after"},
	reflect.TypeOf(GitState{}):       {"root"},
	reflect.TypeOf(BackgroundTask{}): {"command"},
	reflect.TypeOf(TurnEnd{}):        {"prompt_id", "permission_mode", "git", "cost"},
	reflect.TypeOf(TurnCost{}):       {"prompt_id"},
	reflect.TypeOf(Prompt{}):         {"prompt_id", "previous_prompt_id", "prompt_class", "git", "permission_mode"},
	reflect.TypeOf(Human{}):          {"prompt_id", "agent_id", "tool_name", "tool_use_id"},
	reflect.TypeOf(Session{}):        {"agent_type", "project", "git"},
	reflect.TypeOf(Subagent{}):       {"prompt_id", "agent_id", "agent_type"},
	reflect.TypeOf(Batch{}):          {"prompt_id"},
	reflect.TypeOf(Task{}):           {"prompt_id", "subject", "agent_id"},
	reflect.TypeOf(Compact{}):        {"prompt_id"},
	reflect.TypeOf(Instructions{}):   {"prompt_id", "agent_id", "file_path", "trigger_file_path"},
	reflect.TypeOf(Model{}):          {"prompt_id"},
	reflect.TypeOf(WriteObserved{}):  {"path"},
}

// deferredWithheld are capped fields with no withheld key yet, each for a
// stated reason, so the rule below cannot pass by forgetting them:
// OutcomeJoin's tool_use_id is BREADTH's join key, and a join whose key is
// withheld joins nothing, so whether it is written at all is BREADTH's
// call (archive 60). The v4 call record's non-nullable capped fields
// (tool_use_id, transcript_path, permission_mode, tool_name, shape.program)
// are P-11's: their null half needs the v4 structs retyped when P-11
// embeds DeclarationV5 and ExecutionV5.
var deferredWithheld = map[reflect.Type][]string{
	reflect.TypeOf(OutcomeJoin{}): {"tool_use_id"},
}

// TestDEL22g_WithheldKeyBesideEveryCappedOrSwitchableField: every field
// the cap table or a switch can null has a <field>_withheld *string key in
// the same v5 type (1.2.11: "null plus `<field>_withheld: "oversize"`";
// archive 10.2.17: "withheld fields carry `..._withheld: "switched_off"`, so
// 'not recorded by choice' never reads as 'nothing happened'"), and a field
// the v5 type owns is nullable so it can be withheld; every capped key a
// v5 kind carries is listed or deferred, and no _withheld key stands for a
// field the lists do not name (STORE-9).
func TestDEL22g_WithheldKeyBesideEveryCappedOrSwitchableField(t *testing.T) {
	for _, c := range v5Lists {
		types := map[string]reflect.Type{}
		for _, f := range tagCheckedFields(t, c.typ) {
			types[f.Tag.Get("json")] = f.Type
		}
		listed := withheldKeys[c.typ]
		for _, k := range listed {
			if wt, ok := types[k+"_withheld"]; !ok || wt.String() != "*string" {
				t.Errorf("%s: %s can be withheld and has no %s_withheld *string key", c.typ.Name(), k, k)
			}
			if ft, own := types[k]; own && !nullable(ft) {
				t.Errorf("%s.%s is %s, which cannot be null when withheld", c.typ.Name(), k, ft)
			}
		}
		for k := range types {
			if base, ok := strings.CutSuffix(k, "_withheld"); ok && !slices.Contains(listed, base) {
				t.Errorf("%s: %s names no field the cap table or a switch can null", c.typ.Name(), k)
			}
			if slices.Contains(cappedKeys, k) && !slices.Contains(listed, k) && !slices.Contains(deferredWithheld[c.typ], k) {
				t.Errorf("%s: the capped key %s is neither given a _withheld key nor deferred", c.typ.Name(), k)
			}
		}
		for _, k := range deferredWithheld[c.typ] {
			if _, ok := types[k+"_withheld"]; ok {
				t.Errorf("%s: %s_withheld exists; take it off the deferred list", c.typ.Name(), k)
			}
		}
	}
}

// nullable reports a Go type encoding/json writes as null at its zero.
func nullable(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Map:
		return true
	}
	return false
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
