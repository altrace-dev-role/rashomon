// Package corpus holds two hand-labelled classification corpora, drawn from
// public repositories, and the tool that measures where the repository's own
// classifiers disagree with the labels.
//
// paths.jsonl is the path corpus: real repository file paths with the class
// shape.Label would give a Read of that path, labelled by the written rules in
// PATH-LABEL-RULES.txt without running the classifier. shell.jsonl is the
// shell-line corpus: real command lines with the verb class shape.Derive
// gives a Bash call, labelled by SHELL-LABEL-RULES.txt and scrubbed through
// secretshape and the fixture scanner. repos.json records where each row came
// from: URL, pinned commit, licence.
//
// This package only reads the classifiers. It never changes a table; a
// disagreement is evidence for whoever owns the table, not a patch.
package corpus

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/shape"
)

//go:embed repos.json
var reposJSON []byte

//go:embed paths.jsonl
var pathsJSONL []byte

//go:embed shell.jsonl
var shellJSONL []byte

// Repo is one source repository, pinned.
type Repo struct {
	ID        string   `json:"id"`
	URL       string   `json:"url"`
	Commit    string   `json:"commit_sha"`
	Licence   string   `json:"licence"`
	Languages []string `json:"languages"`
}

// PathRow is one labelled repository path. Stratum says how it was sampled:
// dir (stratified over directories), cand (name resembles a class of interest)
// or sens (every sensitive-class file of the repository).
type PathRow struct {
	ID      string `json:"id"`
	Repo    string `json:"repo"`
	Path    string `json:"path"`
	Label   string `json:"label"`
	Rule    string `json:"rule"`
	Stratum string `json:"stratum"`
}

// ShellRow is one labelled shell line. Count is how many sampled sources
// held the identical line before the rows were merged.
type ShellRow struct {
	ID     string `json:"id"`
	Repo   string `json:"repo"`
	Source string `json:"source"`
	File   string `json:"file"`
	Line   string `json:"line"`
	Label  string `json:"label"`
	Rule   string `json:"rule"`
	Count  int    `json:"count"`
}

// LoadRepos returns the repository registry.
func LoadRepos() ([]Repo, error) {
	var r []Repo
	if err := json.Unmarshal(reposJSON, &r); err != nil {
		return nil, fmt.Errorf("repos.json: %w", err)
	}
	return r, nil
}

// LoadPaths returns the path corpus in file order.
func LoadPaths() ([]PathRow, error) { return decodeLines[PathRow](pathsJSONL, "paths.jsonl") }

// LoadShell returns the shell-line corpus in file order.
func LoadShell() ([]ShellRow, error) { return decodeLines[ShellRow](shellJSONL, "shell.jsonl") }

// decodeLines reads one JSON object per line and refuses unknown fields, so a
// column added to a data file without the type learning of it fails the load.
func decodeLines[T any](data []byte, name string) ([]T, error) {
	var out []T
	for n, line := range bytes.Split(data, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var v T
		d := json.NewDecoder(bytes.NewReader(line))
		d.DisallowUnknownFields()
		if err := d.Decode(&v); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", name, n+1, err)
		}
		out = append(out, v)
	}
	return out, nil
}

// Judged is one row after the classifier has answered: what the labeller said
// (Want) and what the classifier said (Got).
type Judged struct {
	ID      string `json:"id"`
	Repo    string `json:"repo"`
	Subject string `json:"subject"`
	Rule    string `json:"rule"`
	Stratum string `json:"stratum"`
	Want    string `json:"want"`
	Got     string `json:"got"`
}

// Pair counts rows the classifier moved from one class to another.
type Pair struct {
	Want string `json:"want"`
	Got  string `json:"got"`
	N    int    `json:"n"`
}

// Slice is the agreement inside one group of rows.
type Slice struct {
	Key   string `json:"key"`
	Total int    `json:"total"`
	Agree int    `json:"agree"`
}

// Summary is the whole measurement: counts, the disagreement rows, and the
// same counts cut by repository, stratum, labelling rule and wanted class.
// Every slice is sorted, so equal inputs give equal bytes.
type Summary struct {
	Total    int      `json:"total"`
	Agree    int      `json:"agree"`
	Disagree []Judged `json:"disagreements"`
	Pairs    []Pair   `json:"pairs"`
	ByRepo   []Slice  `json:"by_repo"`
	ByStrat  []Slice  `json:"by_stratum"`
	ByRule   []Slice  `json:"by_rule"`
	ByWant   []Slice  `json:"by_label"`
}

// ShortCommit is the form of a commit the text report prints: twelve digits.
// The full SHA is in the JSON report; a bare 40-digit run in free text is what
// the fixture scanner reads as a key, and twelve are enough to name a commit.
func ShortCommit(c string) string {
	if len(c) > 12 {
		return c[:12]
	}
	return c
}

// Summarize folds judged rows. It is a pure function of its argument and keeps
// the argument's order for the disagreement list.
func Summarize(rows []Judged) Summary {
	s := Summary{Total: len(rows), Disagree: []Judged{}}
	pairs := map[[2]string]int{}
	cuts := [4]map[string]*Slice{{}, {}, {}, {}}
	for _, r := range rows {
		ok := r.Want == r.Got
		for i, k := range [4]string{r.Repo, r.Stratum, r.Rule, r.Want} {
			c := cuts[i][k]
			if c == nil {
				c = &Slice{Key: k}
				cuts[i][k] = c
			}
			c.Total++
			if ok {
				c.Agree++
			}
		}
		if ok {
			s.Agree++
			continue
		}
		s.Disagree = append(s.Disagree, r)
		pairs[[2]string{r.Want, r.Got}]++
	}
	for k, n := range pairs {
		s.Pairs = append(s.Pairs, Pair{Want: k[0], Got: k[1], N: n})
	}
	sort.Slice(s.Pairs, func(i, j int) bool {
		a, b := s.Pairs[i], s.Pairs[j]
		if a.N != b.N {
			return a.N > b.N
		}
		if a.Want != b.Want {
			return a.Want < b.Want
		}
		return a.Got < b.Got
	})
	flat := func(m map[string]*Slice) []Slice {
		out := []Slice{}
		for _, v := range m {
			out = append(out, *v)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
		return out
	}
	s.ByRepo, s.ByStrat, s.ByRule, s.ByWant = flat(cuts[0]), flat(cuts[1]), flat(cuts[2]), flat(cuts[3])
	return s
}

// judgeKey is any fixed key: the verb class never depends on it, it only keys
// the digest Derive also computes.
var judgeKey = bytes.Repeat([]byte{1}, 32)

// JudgePaths asks shape.Label, for a Read of each path, what it thinks the
// path is. Read is the tool whose file_path shape.Label labels.
func JudgePaths(rows []PathRow) ([]Judged, error) {
	out := make([]Judged, 0, len(rows))
	for _, r := range rows {
		in, err := json.Marshal(map[string]string{"file_path": r.Path})
		if err != nil {
			return nil, err
		}
		out = append(out, Judged{ID: r.ID, Repo: r.Repo, Subject: r.Path, Rule: r.Rule, Stratum: r.Stratum,
			Want: r.Label, Got: shape.Label("Read", in)})
	}
	return out, nil
}

// JudgeShell asks shape.Derive, for a Bash call of each line, for its verb class.
func JudgeShell(rows []ShellRow) ([]Judged, error) {
	out := make([]Judged, 0, len(rows))
	for _, r := range rows {
		in, err := json.Marshal(map[string]string{"command": r.Line})
		if err != nil {
			return nil, err
		}
		out = append(out, Judged{ID: r.ID, Repo: r.Repo, Subject: r.Line, Rule: r.Rule, Stratum: r.Source,
			Want: r.Label, Got: shape.Derive("Bash", in, judgeKey).VerbClass})
	}
	return out, nil
}

// Build runs both classifiers over both corpora and returns the four report
// files by name. classifier is the commit the classifiers were read at; it is
// printed, not interpreted.
func Build(classifier string) (map[string][]byte, error) {
	paths, err := LoadPaths()
	if err != nil {
		return nil, err
	}
	lines, err := LoadShell()
	if err != nil {
		return nil, err
	}
	jp, err := JudgePaths(paths)
	if err != nil {
		return nil, err
	}
	js, err := JudgeShell(lines)
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	for _, c := range []struct {
		name, title, classifierName, strata string
		rows                                []Judged
	}{
		{"paths", "Path corpus", "shape.Label (Read file_path)", "stratum: dir = stratified over directories, cand = name resembles a class of interest, sens = every sensitive-class file", jp},
		{"shell", "Shell-line corpus", "shape.Derive (Bash command) VerbClass", "stratum here is the line's source: workflow, make, script, readme", js},
	} {
		sum := Summarize(c.rows)
		j, err := json.MarshalIndent(struct {
			Corpus     string  `json:"corpus"`
			Classifier string  `json:"classifier"`
			Commit     string  `json:"classifier_commit_sha"`
			Summary    Summary `json:"summary"`
		}{c.title, c.classifierName, classifier, sum}, "", " ")
		if err != nil {
			return nil, err
		}
		files[c.name+".json"] = append(j, '\n')
		files[c.name+".txt"] = []byte(text(c.title, c.classifierName, c.strata, ShortCommit(classifier), sum))
	}
	return files, nil
}

func rate(agree, total int) string {
	if total == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d/%d = %.2f%%", agree, total, 100*float64(agree)/float64(total))
}

func text(title, classifier, strata, commit string, s Summary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: disagreement report\nclassifier: %s\nclassifier commit (first 12 digits; full SHA in the .json): %s\n%s\n\n", title, classifier, commit, strata)
	fmt.Fprintf(&b, "rows: %d\nagree: %s\ndisagree: %d\n\n", s.Total, rate(s.Agree, s.Total), len(s.Disagree))
	fmt.Fprintf(&b, "disagreements by (label -> classifier answer):\n")
	for _, p := range s.Pairs {
		fmt.Fprintf(&b, "  %5d  %s -> %s\n", p.N, p.Want, p.Got)
	}
	for _, c := range []struct {
		name string
		by   []Slice
	}{{"label", s.ByWant}, {"repository", s.ByRepo}, {"stratum", s.ByStrat}, {"labelling rule", s.ByRule}} {
		fmt.Fprintf(&b, "\nagreement by %s:\n", c.name)
		for _, x := range c.by {
			fmt.Fprintf(&b, "  %-28s %s\n", x.Key, rate(x.Agree, x.Total))
		}
	}
	fmt.Fprintf(&b, "\nevery disagreement (id, repo, rule, label -> classifier, subject):\n")
	for _, d := range s.Disagree {
		b.WriteString(ReportLine(d))
	}
	return b.String()
}

// ReportLine is the one line the text report prints for a disagreement,
// newline included. It is exported so cmd/scrubfilter can scan exactly what
// the report will carry, which is not the same text as the data row.
func ReportLine(d Judged) string {
	return fmt.Sprintf("  %s  %s  [%s]  %s -> %s  %s\n", d.ID, d.Repo, d.Rule, d.Want, d.Got, quote(d.Subject))
}

// quote prints a subject on one line, so a multi-line shell row stays one row.
func quote(s string) string {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	_ = e.Encode(s) // a string always encodes
	return strings.TrimSuffix(b.String(), "\n")
}
