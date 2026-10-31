package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/consent"
	"github.com/altrace-dev-role/rashomon/internal/disclose"
)

// bareCommand is "rashomon" and the word after it in running text: a command
// when the word is a verb run dispatches, a sentence about rashomon when it
// is one of proseAfterRashomon.
var bareCommand = regexp.MustCompile(`rashomon ([a-z][a-z.-]*)`)

// proseAfterRashomon are the words the screen and watch's prints put after
// "rashomon" in a sentence rather than a command. The list is closed: a new
// word fails TestTRU2c_ until it is added here, so a verb main lacks cannot
// pass as prose.
var proseAfterRashomon = map[string]bool{
	"adds": true, "binary": true, "copy": true, "does": true, "gave": true, "itself": true,
	"keeps": true, "never": true, "reads": true, "record": true, "records": true, "sends": true,
	"told": true,
}

// TestTRU2c_ScreenAndWatchNameOnlyCommandsMainDispatches: every command the
// consent screen's paragraphs, its unit screens, its closing table and the
// prints of watch and consent name is a verb run dispatches, and every flag
// or sub-command it carries is a word of that verb's lines on the usage
// screen. The same sweep arguments_test runs over the README's table, here
// over the text a person agrees to.
func TestTRU2c_ScreenAndWatchNameOnlyCommandsMainDispatches(t *testing.T) {
	verbs, usage := dispatchedVerbs(t), usageWords(t)
	inv, err := disclose.Default()
	if err != nil {
		t.Fatal(err)
	}
	texts := map[string]string{}
	for i, p := range inv.Paragraphs {
		texts["paragraph "+strconv.Itoa(i)+" ("+p.Owner+", "+p.Cite+")"] = p.Text
	}
	for _, u := range inv.Units {
		texts["unit "+u.Unit] = u.Text
	}
	for _, c := range consent.Commands {
		texts["closing command "+c.Does] = "`" + c.Run + "`"
	}
	for _, file := range []string{"watch.go", "consent.go"} {
		for i, s := range stringLiterals(t, file) {
			texts[file+" string "+strconv.Itoa(i)] = s
		}
	}
	if len(texts) < len(inv.Paragraphs)+len(consent.Commands) {
		t.Fatalf("premise: read %d texts", len(texts))
	}
	for where, text := range texts {
		for _, f := range commandFaults(text, verbs, usage) {
			t.Errorf("%s: %s", where, f)
		}
	}
}

// TestTRU2d_CommandFaultsFindsEachKind: the sweep's own check, fed each
// defect it exists to find and each sentence it must let pass, since a
// removed check never fails on a text that is already right.
func TestTRU2d_CommandFaultsFindsEachKind(t *testing.T) {
	verbs, usage := dispatchedVerbs(t), usageWords(t)
	for text, want := range map[string]bool{
		"`rashomon steer log` shows them":              true,
		"only in rashomon steer log":                   true,
		"rashomon disclose --cost shows this machine":  true,
		"rashomon forget --commands removes it":        true,
		"Turn it off with `rashomon learn off --user`": true,
		"rashomon frobnicates the store":               true,
		"rashomon status counts the sessions":          false,
		"until you run `rashomon spec trust`.":         false,
		"`rashomon detach --install %s`":               false,
		"rashomon records them only if you answer y":   false,
		"`rashomon consent bodies off` stops them":     false,
	} {
		if got := len(commandFaults(text, verbs, usage)) > 0; got != want {
			t.Errorf("commandFaults(%q) found a fault: %v, want %v", text, got, want)
		}
	}
}

// commandFaults is every command text names that run does not dispatch, or
// names with a flag or sub-command its usage lines do not carry. A command in
// backticks is read to the closing backtick; one in running text is read
// while its words are flags or words of the verb's usage lines, and stops at
// the first other word, where the sentence resumes.
func commandFaults(text string, verbs map[string]bool, usage map[string]map[string]bool) []string {
	var faults []string
	for i, part := range strings.Split(text, "`") {
		code := i%2 == 1
		if code {
			if rest, ok := strings.CutPrefix(part, "rashomon "); ok {
				faults = append(faults, commandFault(strings.Fields(rest), true, verbs, usage)...)
			}
			continue
		}
		for _, at := range bareCommand.FindAllStringIndex(part, -1) {
			words := strings.Fields(part[at[0]:])[1:]
			faults = append(faults, commandFault(words, false, verbs, usage)...)
		}
	}
	return faults
}

// commandFault checks one command's words after "rashomon"; strict is a
// command in backticks, every word of which is the command's.
func commandFault(words []string, strict bool, verbs map[string]bool, usage map[string]map[string]bool) []string {
	verb := strings.Trim(words[0], ".,;:)\"")
	if !verbs[verb] {
		if !strict && proseAfterRashomon[verb] {
			return nil
		}
		return []string{"names `rashomon " + verb + "`, a verb main.go does not dispatch"}
	}
	for _, w := range words[1:] {
		w = strings.Trim(w, ".,;:()\"")
		if w == "" || strings.ContainsAny(w, "%<ABCDEFGHIJKLMNOPQRSTUVWXYZ") || usage[verb][w] {
			continue
		}
		if strict || strings.HasPrefix(w, "-") {
			return []string{"names `rashomon " + verb + " ... " + w + "`, which the usage screen does not give " + verb}
		}
		break
	}
	return nil
}

// dispatchedVerbs is every case of run's switch on args[0], read from
// main.go, plus every case of verbNamed in verbs.go (STATUS-VERBS1a's
// Law-10 path: new verbs cost main.go no line). A verb added in either
// place is known here without a second list.
func dispatchedVerbs(t *testing.T) map[string]bool {
	t.Helper()
	verbs := map[string]bool{}
	collectSwitchStrings := func(file, fnName string, tagPred func(ast.Expr) bool) {
		t.Helper()
		f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Name.Name != fnName {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				sw, ok := n.(*ast.SwitchStmt)
				if !ok {
					return true
				}
				if !tagPred(sw.Tag) {
					return true
				}
				for _, s := range sw.Body.List {
					for _, e := range s.(*ast.CaseClause).List {
						if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.STRING {
							v, _ := strconv.Unquote(lit.Value)
							verbs[v] = true
						}
					}
				}
				return false
			})
		}
	}
	collectSwitchStrings("main.go", "run", func(tag ast.Expr) bool {
		ix, ok := tag.(*ast.IndexExpr)
		return ok && identName(ix.X) == "args"
	})
	collectSwitchStrings("verbs.go", "verbNamed", func(tag ast.Expr) bool {
		return identName(tag) == "name"
	})
	for _, want := range []string{"watch", "detach", "purge", "status", "report", "spec"} {
		if !verbs[want] {
			t.Fatalf("premise: run's switch does not dispatch %q (read %d verbs)", want, len(verbs))
		}
	}
	return verbs
}

// identName is the name of an identifier expression, or "".
func identName(e ast.Expr) string {
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// usageWords is, per verb, every word of its lines on the usage screen: the
// command lines and the description under them, up to the next command.
func usageWords(t *testing.T) map[string]map[string]bool {
	t.Helper()
	var b bytes.Buffer
	usage(&b)
	words := map[string]map[string]bool{}
	verb := ""
	for _, l := range strings.Split(b.String(), "\n") {
		if rest, ok := strings.CutPrefix(l, "  rashomon "); ok {
			verb, _, _ = strings.Cut(rest, " ")
		} else if !strings.HasPrefix(l, "  ") {
			verb = ""
		}
		if verb == "" {
			continue
		}
		if words[verb] == nil {
			words[verb] = map[string]bool{}
		}
		for _, w := range strings.FieldsFunc(l, func(r rune) bool { return strings.ContainsRune(" []|(),;:", r) }) {
			words[verb][w] = true
		}
	}
	if !words["forget"]["--bodies"] || !words["spec"]["trust"] || !words["consent"]["off"] {
		t.Fatalf("premise: the usage screen's words were not read per verb: %v", slices.Sorted(maps.Keys(words)))
	}
	return words
}

// stringLiterals is every string literal in file, unquoted.
func stringLiterals(t *testing.T, file string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if s, err := strconv.Unquote(lit.Value); err == nil {
				out = append(out, s)
			}
		}
		return true
	})
	if len(out) == 0 {
		t.Fatalf("premise: %s has no string literals", file)
	}
	return out
}
