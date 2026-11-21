package disclose

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
)

// What the session log keeps (W-M2.11 step 1d): the graph/*.jsonl row's
// Holds names each field a line of the log stores, the attributes of a node
// and of an edge by key, and the fields the log keeps empty are empty in the
// logs the real chains write.

// slLine is a line of the session log as the loader reads it.
type slLine struct {
	graph.Header
	graph.Record
}

// slLeafTypes are the struct types a line keeps whole, one leaf each.
var slLeafTypes = []reflect.Type{reflect.TypeFor[graph.Rev](), reflect.TypeFor[graph.Prov]()}

// slAttrKeys is each key a node's attributes take in the log: the group
// marker's, the Go overlay's file and symbol nodes', the Python overlay's
// module of a file and line of a declaration, and the mark of a group cut at
// the cap. Typed from the row's Holds, never read from a reader's list.
var slAttrKeys = []string{"session_id", "file", "kind", "seq", "tool_use_id", "digest", "count", "carried", "of_seq",
	"lang", "name", "symkind", "sig", "doc", "calls", "_cut", "module", "line"}

// slEdgeAttrKeys is each key an edge's attributes take in the log: the line
// of a call, which the Python overlay's log keeps. Typed from the row's
// Holds, never read from a reader's list.
var slEdgeAttrKeys = []string{"line"}

// slLeaves is each leaf of typ as its JSON spells it, under prefix, as
// cbLeaves in cmd/rashomon walks the conflict base: an embedded struct with
// no name adds none, a pointer or a slice is its element, a struct is its
// fields unless slLeafTypes keeps it whole, a node's attributes are one leaf
// per slAttrKeys key and an edge's one per slEdgeAttrKeys key, and anything
// else (a string, a number, a map) is one leaf.
func slLeaves(typ reflect.Type, prefix string) []string {
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
		typ = typ.Elem()
	}
	if prefix == "Node.Attrs" || prefix == "Edge.Attrs" {
		keys := slAttrKeys
		if prefix == "Edge.Attrs" {
			keys = slEdgeAttrKeys
		}
		var out []string
		for _, k := range keys {
			out = append(out, prefix+"."+k)
		}
		return out
	}
	if typ.Kind() != reflect.Struct || slices.Contains(slLeafTypes, typ) {
		return []string{prefix}
	}
	var out []string
	for i := range typ.NumField() {
		f := typ.Field(i)
		if !f.IsExported() && !f.Anonymous {
			continue
		}
		tag := f.Tag
		name, _, _ := strings.Cut(tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if f.Anonymous && name == "" {
			out = append(out, slLeaves(f.Type, prefix)...)
			continue
		}
		if name == "" {
			name = f.Name
		}
		if prefix != "" {
			name = prefix + "." + name
		}
		out = append(out, slLeaves(f.Type, name)...)
	}
	return out
}

// slHolds is the Holds words for each leaf of a line, "" for a leaf the log
// keeps empty (slEmpty).
var slHolds = map[string]string{
	"schema":                 "its format's version",
	"repo":                   "the repository's id",
	"rev.commit":             "the graph's commit",
	"producers":              "its writer's name and version",
	"records":                "",
	"refused":                "",
	"refused_files.path":     "",
	"refused_files.line":     "",
	"refused_files.reason":   "",
	"left_out":               "",
	"left_out_dirs.path":     "",
	"left_out_dirs.why":      "",
	"Node.ID":                "each record by its ids",
	"Node.Kind":              "its kind or predicate",
	"Node.Attrs.session_id":  "the session",
	"Node.Attrs.file":        "the file's path in the repository",
	"Node.Attrs.kind":        "which of the two it is",
	"Node.Attrs.seq":         "the call's sequence number",
	"Node.Attrs.tool_use_id": "the call's id",
	"Node.Attrs.digest":      "the file's SHA-256",
	"Node.Attrs.count":       "the count of its records",
	"Node.Attrs.carried":     "the kinds of link its reader records",
	"Node.Attrs.of_seq":      "the sequence number of the one from its input",
	"Node.Attrs.lang":        "language (go)",
	"Node.Attrs.name":        "by name and kind",
	"Node.Attrs.symkind":     "by name and kind",
	"Node.Attrs.sig":         "",
	"Node.Attrs.doc":         "the first sentence of its doc comment",
	"Node.Attrs.calls":       "the names a function or method calls",
	"Node.Attrs._cut":        "whether its declarations were cut at the cap",
	"Node.Attrs.module":      "the module of the file",
	"Node.Attrs.line":        "the line of each declaration",
	"Node.Tier":              "its tier, confidence and status",
	"Node.Conf":              "its tier, confidence and status",
	"Node.Status":            "its tier, confidence and status",
	"Node.Rev":               "the commit and sequence number it belongs to",
	"Node.Prov":              "",
	"Edge.Src":               "each record by its ids",
	"Edge.Pred":              "its kind or predicate",
	"Edge.Dst":               "each record by its ids",
	"Edge.Attrs.line":        "the line of each declaration and of each call",
	"Edge.Tier":              "its tier, confidence and status",
	"Edge.Conf":              "its tier, confidence and status",
	"Edge.Status":            "its tier, confidence and status",
	"Edge.Rev":               "the commit and sequence number it belongs to",
	"Edge.Prov":              "",
	"Edge.Certain":           "",
}

// slEmpty is the leaves the log keeps empty, sorted: a function's signature,
// where a reader read a record, an edge's Certain, the header's record
// count, its refused files (an overlay refuses none) and its directories
// left out (an overlay walks none).
var slEmpty = []string{"Edge.Certain", "Edge.Prov", "Node.Attrs.sig", "Node.Prov", "left_out",
	"left_out_dirs.path", "left_out_dirs.why", "records", "refused", "refused_files.line", "refused_files.path",
	"refused_files.reason"}

// slRow is the session log's inventory row.
func slRow(t *testing.T) FileRule {
	t.Helper()
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	var rows []FileRule
	for _, f := range inv.Files {
		if f.Base == "graph" && f.Pattern == "*.jsonl" {
			rows = append(rows, f)
		}
	}
	if len(rows) != 1 {
		t.Fatalf("%d session log rows, want 1", len(rows))
	}
	return rows[0]
}

// slHoldsFaults is each leaf of typ that slHolds does not name, and each
// whose words holds lacks.
func slHoldsFaults(typ reflect.Type, holds string) []string {
	var out []string
	for _, leaf := range slLeaves(typ, "") {
		words, ok := slHolds[leaf]
		switch {
		case !ok:
			out = append(out, "field "+leaf+" is stored and named by no Holds words")
		case words != "" && !strings.Contains(holds, words):
			out = append(out, "field "+leaf+": the Holds lacks "+strconv.Quote(words))
		}
	}
	return out
}

// slWitnessLine is a line with one field more.
type slWitnessLine struct {
	slLine
	Extra string `json:"extra"`
}

// TestSessionLog_HoldsNamesEachStoredField: every leaf of a session log line
// (graph.Header, and graph.Record's Node and Edge with Rev and Prov taken
// whole and the attributes of each by key) has its words in the
// row's Holds, or is one the log keeps empty, and the row's NeverHolds names
// the signature. Witnesses: with no Holds text every named leaf faults, and
// one field more gives exactly one fault, naming it.
func TestSessionLog_HoldsNamesEachStoredField(t *testing.T) {
	row := slRow(t)
	typ := reflect.TypeFor[slLine]()
	if f := slHoldsFaults(typ, row.Holds); len(f) != 0 {
		t.Errorf("the Holds does not name what the log stores:\n%s", strings.Join(f, "\n"))
	}
	if !strings.Contains(row.NeverHolds, "a function's signature") {
		t.Errorf("the row's NeverHolds does not name a function's signature: %q", row.NeverHolds)
	}
	leaves := slLeaves(typ, "")
	if len(leaves) != len(slHolds) {
		t.Errorf("%d leaves, %d named: %v", len(leaves), len(slHolds), leaves)
	}
	var empty []string
	named := 0
	for leaf, words := range slHolds {
		if !slices.Contains(leaves, leaf) {
			t.Errorf("slHolds names %s, which a line does not store", leaf)
		}
		if words == "" {
			empty = append(empty, leaf)
		} else {
			named++
		}
	}
	slices.Sort(empty)
	if !slices.Equal(empty, slEmpty) {
		t.Errorf("leaves kept empty %v, want %v", empty, slEmpty)
	}
	if n := len(slHoldsFaults(typ, "")); n != named {
		t.Errorf("witness: %d faults against no Holds text, want %d", n, named)
	}
	if f := slHoldsFaults(reflect.TypeFor[slWitnessLine](), row.Holds); len(f) != 1 || !strings.HasPrefix(f[0], "field extra ") {
		t.Errorf("witness: one field more gives faults %q, want exactly one naming extra", f)
	}
}

// slZero is whether v, a decoded JSON value, is its type's zero: null, "",
// 0, false, an empty list, or an object whose every value is zero.
func slZero(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case float64:
		return x == 0
	case bool:
		return !x
	case []any:
		return len(x) == 0
	case map[string]any:
		for _, e := range x {
			if !slZero(e) {
				return false
			}
		}
		return true
	}
	return false
}

// slJSONPaths adds each key path of v, a decoded JSON value, under prefix to
// out, stopping at a path slHolds names (a map key, Rev or Prov whole), and
// adds to faults each path kept empty whose value is not.
func slJSONPaths(v any, prefix string, out map[string]bool, faults *[]string) {
	if words, ok := slHolds[prefix]; ok {
		out[prefix] = true
		if words == "" && !slZero(v) {
			*faults = append(*faults, fmt.Sprintf("%s holds %v", prefix, v))
		}
		return
	}
	switch x := v.(type) {
	case nil:
	case map[string]any:
		for k, e := range x {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			slJSONPaths(e, p, out, faults)
		}
	case []any:
		for _, e := range x {
			slJSONPaths(e, prefix, out, faults)
		}
	default:
		out[prefix] = true
	}
}

// slSource is a Go file with a doc comment on a function whose parameter
// names are a planted word and which calls another, then more constants than
// the log keeps of one group, so the group is cut at the cap.
func slSource() []byte {
	var b strings.Builder
	b.WriteString("package pkg\n\nimport \"strings\"\n\n// Joined joins its words with sep. It says more.\n" +
		"func Joined(slsigword []string, sep string) string {\n\treturn strings.Join(slsigword, sep)\n}\n\nconst (\n")
	for i := range graph.MaxOverlayRecords {
		fmt.Fprintf(&b, "\tc%d = %d\n", i, i)
	}
	b.WriteString(")\n")
	return []byte(b.String())
}

// slPySource is a Python file with a function whose parameter names are a
// planted word and which calls another function of the file.
func slPySource() []byte {
	return []byte("def joined(slsigword, sep):\n    return helper(slsigword)\n\n\ndef helper(words):\n    return words\n")
}

// slKeep is a copy of recs whose every node and edge holds only the
// attributes kept names for its kind, each entry "<record> <kind> <name>"
// (lang.KeptFrom): the rule the hook's writer applies to a stating reader's
// records before the build (keepAttrs in internal/hook, not exported). recs
// is not written.
func slKeep(recs []graph.Record, kept []string) []graph.Record {
	cut := func(part, kind string, attrs map[string]string) map[string]string {
		var out map[string]string
		for name, val := range attrs {
			if !slices.Contains(kept, part+" "+kind+" "+name) {
				continue
			}
			if out == nil {
				out = map[string]string{}
			}
			out[name] = val
		}
		return out
	}
	out := make([]graph.Record, 0, len(recs))
	for _, rec := range recs {
		switch {
		case rec.Node != nil:
			node := *rec.Node
			node.Attrs = cut("node", node.Kind, node.Attrs)
			out = append(out, graph.Record{Node: &node})
		case rec.Edge != nil:
			edge := *rec.Edge
			edge.Attrs = cut("edge", string(edge.Pred), edge.Attrs)
			out = append(out, graph.Record{Edge: &edge})
		default:
			out = append(out, rec)
		}
	}
	return out
}

// slPyLog is the log the Python chain writes of slPySource with kept as the
// reader's list: pylite's Overlay, slKeep, BuildOverlayGroup and
// AppendOverlayGroup. Premise: one of the reader's records carries a sig
// with the planted word.
func slPyLog(t *testing.T, kept []string) []byte {
	t.Helper()
	const rel = "app/a.py"
	src := slPySource()
	reader := pylite.Resolver{Units: []pylite.Unit{{Path: "app"}}}
	recs, err := reader.Overlay(rel, src, graph.RevCommit("c1"), 3)
	if err != nil {
		t.Fatal(err)
	}
	sigs := 0
	for _, rec := range recs {
		if rec.Node != nil && strings.Contains(rec.Node.Attrs["sig"], "slsigword") {
			sigs++
		}
	}
	if sigs != 1 {
		t.Fatalf("premise: %d of the Python reader's records carry the planted sig, want 1", sigs)
	}
	group := graph.OverlayGroup{Session: "s1", ToolUseID: "toolu_slpy", Kind: graph.OverlayLanded, File: graph.FileID(rel),
		Digest: graph.ContentDigest(src), Commit: "c1", Seq: 3, OfSeq: 2, Carried: reader.OverlayCarried(), Recs: slKeep(recs, kept)}
	sessLog := graph.OpenSessionLog(t.TempDir(), "github.com/x/y", graph.RevCommit("c1"), true)
	if err := sessLog.AppendOverlayGroup(graph.BuildOverlayGroup(group), nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(sessLog.Path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// slLogFaults is each fault of the lines of logs, read as one, against
// slHolds, sorted: a leaf kept empty that holds a value, a key path slHolds
// does not name, and a leaf it names with words that no line writes.
func slLogFaults(t *testing.T, logs ...[]byte) []string {
	t.Helper()
	paths := map[string]bool{}
	var held, faults []string
	for _, data := range logs {
		for _, line := range bytes.Split(bytes.TrimSuffix(data, []byte{'\n'}), []byte{'\n'}) {
			var v any
			if err := json.Unmarshal(line, &v); err != nil {
				t.Fatalf("a log line does not read back: %v", err)
			}
			slJSONPaths(v, "", paths, &held)
		}
	}
	for _, f := range held {
		faults = append(faults, "the written log keeps a field it keeps empty: "+f)
	}
	for p := range paths {
		if _, ok := slHolds[p]; !ok {
			faults = append(faults, "the written log stores "+p+", which slHolds does not name")
		}
	}
	for leaf, words := range slHolds {
		if words != "" && !paths[leaf] {
			faults = append(faults, "witness: the written log has no "+leaf)
		}
	}
	slices.Sort(faults)
	return faults
}

// TestSessionLog_WrittenLogKeepsNoSignatureOrProvenance: the real chains,
// Go's (gopg's Overlay, BuildOverlayGroup, AppendOverlayGroup) and Python's
// (pylite's Overlay, its records cut to the reader's list of the attributes
// kept as the hook's writer cuts them, then the same two), write logs in
// which, read as one, every key path is one slHolds names, every leaf it
// names with words is written, every leaf kept empty is empty, and in each
// the planted signature and the reader's name reach no line. Witnesses: the
// Go resolver's records carry a sig with the planted word and a Prov, and
// keep both after the build; the reader's list with one entry more, the
// argc of a call, gives exactly one fault, naming it.
func TestSessionLog_WrittenLogKeepsNoSignatureOrProvenance(t *testing.T) {
	const rel = "pkg/a.go"
	src := slSource()
	recs, err := gopg.Resolver{}.Overlay(rel, src, graph.RevCommit("c1"), 3)
	if err != nil {
		t.Fatal(err)
	}
	carried := func() (sigs, provs int) {
		for _, r := range recs {
			if r.Node != nil && strings.Contains(r.Node.Attrs["sig"], "slsigword") {
				sigs++
			}
			if r.Node != nil && r.Node.Prov.Extractor != "" || r.Edge != nil && r.Edge.Prov.Extractor != "" {
				provs++
			}
		}
		return sigs, provs
	}
	if sigs, provs := carried(); sigs != 1 || provs != len(recs) {
		t.Fatalf("witness: %d symbols carry the planted sig and %d of %d records a Prov, want 1 and all", sigs, provs, len(recs))
	}
	g := graph.OverlayGroup{Session: "s1", ToolUseID: "toolu_sl", Kind: graph.OverlayLanded, File: graph.FileID(rel),
		Digest: graph.ContentDigest(src), Commit: "c1", Seq: 3, OfSeq: 2, Carried: gopg.Resolver{}.OverlayCarried(), Recs: recs}
	l := graph.OpenSessionLog(t.TempDir(), "github.com/x/y", graph.RevCommit("c1"), true)
	if err := l.AppendOverlayGroup(graph.BuildOverlayGroup(g), nil); err != nil {
		t.Fatal(err)
	}
	if sigs, provs := carried(); sigs != 1 || provs != len(recs) {
		t.Errorf("after the build %d symbols carry the sig and %d of %d records a Prov: the group in memory lost them", sigs, provs, len(recs))
	}
	data, err := os.ReadFile(l.Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"slsigword", "go/parser"} {
		if bytes.Contains(data, []byte(w)) {
			t.Errorf("the log holds %q", w)
		}
	}
	pyData := slPyLog(t, pylite.Resolver{}.SessionKeptAttrs())
	for _, w := range []string{"slsigword", "pylite"} {
		if bytes.Contains(pyData, []byte(w)) {
			t.Errorf("the Python log holds %q", w)
		}
	}
	for _, f := range slLogFaults(t, data, pyData) {
		t.Error(f)
	}
	argc := append(slices.Clone(pylite.Resolver{}.SessionKeptAttrs()), "edge calls argc")
	if f := slLogFaults(t, data, slPyLog(t, argc)); len(f) != 1 || f[0] != "the written log stores Edge.Attrs.argc, which slHolds does not name" {
		t.Errorf("witness: the reader's list with the argc of a call gives faults %q, want exactly one naming Edge.Attrs.argc", f)
	}
	if n := len(bytes.Split(bytes.TrimSuffix(data, []byte{'\n'}), []byte{'\n'})); n < graph.MaxOverlayRecords {
		t.Errorf("witness: %d lines, want the cut group's %d and more", n, graph.MaxOverlayRecords)
	}
}
