package report

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// hostBearing is the declared answer to "which fields of a Session can hold a
// hostname", by JSON path.
//
// A DECLARED VOCABULARY, like declarationKeys and shape.Labels(), and for the
// same reason: the failure it guards is a field added later that nobody
// classifies. The test below walks the real Session type and fails when the
// reachable set and this list disagree IN EITHER DIRECTION -- so a new section
// cannot be added without someone deciding, in writing, whether it can carry a
// host.
//
// This exists because Redact's own comment -- "every field that can hold a
// hostname is covered here... the risk is that a LATER field is added and
// nobody adds it here" -- described the defect correctly and then suffered it:
// Session gained the sandbox section with hostname lists, Redact was not
// taught about them, and --redact published internal hostnames in clear under
// a legend asserting they were digested. A legend is what a user checks
// INSTEAD of the hostnames, so that is worse than no redaction at all.
var hostBearing = map[string]bool{
	"destinations.hosts[].host":             true,
	"destinations.wire_only[]":              true,
	"destinations.client_plane[]":           true,
	"destinations.declared_not_observed[]":  true,
	"destinations.not_observable[]":         true,
	"destinations.novelty.hosts[]":          true,
	"chains.prompts[].transcript_path":      true,
	"chains.prompts[].links[].hosts[].host": true,
	"chains.prompts[].links[].ssh_hosts[]":  true,
	"chains.unattributed[].hosts[].host":    true,
	"chains.unattributed[].ssh_hosts[]":     true,
	"chains.dropped[].hosts[].host":         true,
	"chains.dropped[].ssh_hosts[]":          true,
	"nono.allowed[]":                        true,
	"nono.denied[]":                         true,
	"nono.saw_what_the_proxy_did_not[]":     true,
	"nono.plain_http[]":                     true,
	"nono.denied_but_reached[]":             true,
	"nono.proxy_saw_what_it_did_not[]":      true,
	"transcripts[].path":                    true,

	// NOT host-bearing, each decided rather than defaulted. Ids, reason codes,
	// tool names, counts-as-strings and enum values. If one of these ever starts
	// carrying a hostname, the entry moves to true and Redact gains a line.
	"account.text":                                     false,
	"chains.dropped[].hosts[].state":                   false,
	"chains.dropped[].outcome":                         false,
	"chains.dropped[].outcomes[]":                      false,
	"chains.dropped[].program":                         false,
	"chains.dropped[].tool_name":                       false,
	"chains.dropped[].tool_use_id":                     false,
	"chains.dropped[].verb_class":                      false,
	"chains.prompts[].links[].hosts[].state":           false,
	"chains.prompts[].links[].outcome":                 false,
	"chains.prompts[].links[].outcomes[]":              false,
	"chains.prompts[].links[].program":                 false,
	"chains.prompts[].links[].tool_name":               false,
	"chains.prompts[].links[].tool_use_id":             false,
	"chains.prompts[].links[].verb_class":              false,
	"chains.prompts[].prompt_id":                       false,
	"chains.unattributed[].hosts[].state":              false,
	"chains.unattributed[].outcome":                    false,
	"chains.unattributed[].outcomes[]":                 false,
	"chains.unattributed[].program":                    false,
	"chains.unattributed[].tool_name":                  false,
	"chains.unattributed[].tool_use_id":                false,
	"chains.unattributed[].verb_class":                 false,
	"coverage.hook_entry_at_end":                       false,
	"coverage.hook_entry_at_start":                     false,
	"coverage.reasons[]":                               false,
	"coverage.state":                                   false,
	"declarations.dropped[]":                           false,
	"declarations.unterminated[]":                      false,
	"declarations.without_execution[].permission_mode": false,
	"declarations.without_execution[].tool_use_id":     false,
	"destinations.hosts[].actions[]":                   false,
	"destinations.hosts[].first_seen":                  false,
	"destinations.hosts[].last_seen":                   false,
	"destinations.hosts[].reasons[]":                   false,
	"destinations.novelty.reason":                      false,
	"destinations.proxy_note":                          false,
	"destinations.proxy_on_path":                       false,
	"destinations.reason":                              false,
	"destinations.rewritten[].program":                 false,
	"destinations.rewritten[].tool_name":               false,
	"destinations.rewritten[].tool_use_id":             false,
	"destinations.rewritten[].verb_class":              false,
	"families.families[].name":                         false,
	"families.families[].programs[]":                   false,
	"families.families[].status":                       false,
	"families.not_exercised[]":                         false,
	"families.not_observable[]":                        false,
	"families.reason":                                  false,
	"gaps[].host_digest":                               false,
	"gaps[].reason":                                    false,
	"gaps[].session_id":                                false,
	"gaps[].type":                                      false,
	"install_id":                                       false,
	"nono.reason":                                      false,
	"session_id":                                       false,
	"silent_failures.absent_words[]":                   false,
	"subagents[].agent_id":                             false,
	"subagents[].agent_type":                           false,
	"transcripts[].declared_without_result[]":          false,
	"transcripts[].denied_by_user[]":                   false,
	"transcripts[].executed_but_unrecorded[]":          false,
	"transcripts[].missing_from_store[]":               false,
	"transcripts[].missing_from_transcript[]":          false,
}

// TestRedact_TheHostBearingSetIsDeclared fails when the Session type grows or
// loses a string-shaped field and nobody says whether it can hold a host.
//
// It is the half that survives the next section. An enumeration of today's
// fields would have passed on the day the bug shipped.
func TestRedact_TheHostBearingSetIsDeclared(t *testing.T) {
	found := map[string]bool{}
	walkStringPaths(reflect.TypeOf(Session{}), "", found, 0)

	var undeclared, stale []string
	for p := range found {
		if _, ok := hostBearing[p]; !ok {
			undeclared = append(undeclared, p)
		}
	}
	for p := range hostBearing {
		if !found[p] {
			stale = append(stale, p)
		}
	}
	sort.Strings(undeclared)
	sort.Strings(stale)

	if len(undeclared) > 0 {
		t.Errorf("%d string-shaped field(s) on Session are not classified. For each, decide "+
			"whether it can hold a hostname: if it can, digest it in Redact AND add it here "+
			"as true; if it cannot, add it here as false. This is the check that would have "+
			"caught the sandbox section shipping four host lists past Redact.\n  %s",
			len(undeclared), strings.Join(undeclared, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("%d declared field(s) no longer exist on Session; remove them so this list "+
			"keeps describing the type:\n  %s", len(stale), strings.Join(stale, "\n  "))
	}
}

// TestRedact_EveryDeclaredHostFieldIsDigested plants a canary in each field the
// list above marks host-bearing and asserts none survives, in JSON or text.
func TestRedact_EveryDeclaredHostFieldIsDigested(t *testing.T) {
	const canary = "canary-host.internal.example"

	var planted int
	var sess Session
	plantAt(reflect.ValueOf(&sess).Elem(), "", canary, &planted, 0)
	if planted == 0 {
		t.Fatal("premise: planted nothing, so this test proves nothing")
	}
	t.Logf("planted the canary in %d declared host-bearing fields", planted)

	rep := &Report{Sessions: []Session{sess}}
	red := Redact(rep, []byte("install-key"))

	body, err := json.Marshal(red)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), canary) {
		t.Errorf("a hostname survived --redact in JSON:\n%s", locate(string(body), canary))
	}
	var text strings.Builder
	// A named proxy store and the chain listing: the two options that put
	// every host-bearing section on the page. Without the store the proxy
	// block collapses to one line, and the canary check would pass against
	// sections that never rendered.
	if err := Text(&text, red, WithChain(), WithNamedProxyStore("causal.db")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text.String(), canary) {
		t.Errorf("a hostname survived --redact in text:\n%s", locate(text.String(), canary))
	}

	// Redact returns a COPY. A shared backing array would write digests into
	// the report the caller still believes is unredacted.
	orig, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(orig), canary) {
		t.Error("the original report lost its hostnames; Redact wrote through a shared array")
	}
}

// walkStringPaths records the JSON path of every string and []string reachable
// from t.
func walkStringPaths(t reflect.Type, prefix string, out map[string]bool, depth int) {
	if depth > 8 {
		return
	}
	switch t.Kind() {
	case reflect.Ptr:
		walkStringPaths(t.Elem(), prefix, out, depth+1)
	case reflect.Slice:
		if t.Elem().Kind() == reflect.String {
			out[prefix+"[]"] = true
			return
		}
		walkStringPaths(t.Elem(), prefix+"[]", out, depth+1)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				continue
			}
			p := name
			if prefix != "" {
				p = prefix + "." + name
			}
			if f.Type.Kind() == reflect.String {
				out[p] = true
				continue
			}
			walkStringPaths(f.Type, p, out, depth+1)
		}
	}
}

// plantAt writes host into each declared host-bearing field.
func plantAt(v reflect.Value, prefix, host string, n *int, depth int) {
	if depth > 8 {
		return
	}
	switch v.Kind() {
	case reflect.Ptr:
		if v.CanSet() && v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		if !v.IsNil() {
			plantAt(v.Elem(), prefix, host, n, depth+1)
		}
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.String {
			if hostBearing[prefix+"[]"] && v.CanSet() {
				v.Set(reflect.ValueOf([]string{host}))
				*n++
			}
			return
		}
		elem := reflect.New(v.Type().Elem()).Elem()
		plantAt(elem, prefix+"[]", host, n, depth+1)
		if v.CanSet() {
			v.Set(reflect.Append(reflect.MakeSlice(v.Type(), 0, 1), elem))
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)
			if !f.IsExported() {
				continue
			}
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				continue
			}
			p := name
			if prefix != "" {
				p = prefix + "." + name
			}
			if v.Field(i).Kind() == reflect.String {
				if hostBearing[p] && v.Field(i).CanSet() {
					v.Field(i).SetString(host)
					*n++
				}
				continue
			}
			plantAt(v.Field(i), p, host, n, depth+1)
		}
	}
}

// locate returns the text surrounding the first hit, so a failure names where
// the leak is rather than only that there is one.
func locate(hay, needle string) string {
	i := strings.Index(hay, needle)
	if i < 0 {
		return ""
	}
	start, end := i-120, i+len(needle)+120
	if start < 0 {
		start = 0
	}
	if end > len(hay) {
		end = len(hay)
	}
	return "..." + hay[start:end] + "..."
}
