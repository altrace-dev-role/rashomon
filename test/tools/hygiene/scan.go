package hygiene

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/secretshape"
)

// Finding is one thing a fixture must not carry: where, and what shape. It
// never prints the value, because a scan that printed the credential it found
// would publish it in the CI log it exists to keep it out of. Nor does String
// print the fingerprint: a digest of a short password is a dictionary away
// from the password.
type Finding struct {
	Path string // relative to the root walked; an archive member is archive!member
	Line int    // 1-based; 0 for the file as a whole
	What string
	// Fingerprint names the exact value: the first 16 hex digits of SHA-256
	// over the path, the shape and the value. reviewed.txt excuses a finding
	// by it, and scrub -review prints it for the person deciding.
	Fingerprint string
}

func (f Finding) String() string { return fmt.Sprintf("%s:%d: %s", f.Path, f.Line, f.What) }

func fingerprint(path, what, value string) string {
	sum := sha256.Sum256([]byte(path + "\x00" + what + "\x00" + value))
	return hex.EncodeToString(sum[:8])
}

// Reviewed is where a person records a finding they have read and judged
// neither a secret nor an account's shape, relative to the root walked.
const Reviewed = "test/tools/hygiene/reviewed.txt"

// Walk reads every file under root that this repository keeps as a fixture or
// a captured payload: everything inside a directory named testdata, every
// NDJSON and JSON Lines file and every environment file wherever it is, and
// every JSON, text, log and archive file under test/ or experiments/. It
// returns the files it read, so a clean result can be told from a walk that
// read nothing, and what Scan found in them that Reviewed does not excuse.
func Walk(root string) (scanned []string, found []Finding, err error) {
	var all []Finding
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			// The dot-directories at the top hold tooling state, not
			// fixtures: .git, and .claude, whose worktrees/ are other
			// checkouts of this repository with walks of their own.
			if rel != "." && !strings.Contains(rel, "/") && strings.HasPrefix(rel, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !selected(rel) {
			return nil
		}
		if !d.Type().IsRegular() {
			what := "not a regular file, so not read: commit the fixture itself"
			all = append(all, Finding{rel, 0, what, fingerprint(rel, what, "")})
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		scanned = append(scanned, rel)
		all = append(all, Scan(rel, data)...)
		return nil
	})
	if err != nil {
		return scanned, nil, err
	}
	return scanned, excuse(root, all), nil
}

// excuse drops the findings Reviewed names, by path and fingerprint, so an
// entry excuses one value in one file: the same value changed, or another
// beside it, is a finding again. An entry that excuses nothing is a finding
// too, so the list cannot fill with entries nobody can check.
func excuse(root string, all []Finding) []Finding {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(Reviewed)))
	if err != nil {
		return all
	}
	entries := map[string]int{}
	var order []string
	for i, ln := range strings.Split(string(data), "\n") {
		f := strings.Fields(ln)
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		if len(f) < 3 {
			all = append(all, Finding{Reviewed, i + 1, "an entry that is not <path> <fingerprint> <why>", ""})
			continue
		}
		entries[f[0]+" "+f[1]] = i + 1
		order = append(order, f[0]+" "+f[1])
	}
	var kept []Finding
	used := map[string]bool{}
	for _, f := range all {
		if key := f.Path + " " + f.Fingerprint; entries[key] > 0 {
			used[key] = true
			continue
		}
		kept = append(kept, f)
	}
	for _, key := range order {
		if !used[key] {
			kept = append(kept, Finding{Reviewed, entries[key], "an entry that excuses nothing: delete it", ""})
		}
	}
	return kept
}

func selected(rel string) bool {
	parts := strings.Split(rel, "/")
	for _, dir := range parts[:len(parts)-1] {
		if dir == "testdata" {
			return true
		}
	}
	if environmentFile(rel) {
		return true
	}
	switch strings.ToLower(path.Ext(rel)) {
	case ".ndjson", ".jsonl":
		return true
	}
	if parts[0] != "test" && parts[0] != "experiments" {
		return false
	}
	switch strings.ToLower(path.Ext(rel)) {
	case ".json", ".txt", ".log":
		return true
	}
	return archiveKind(rel) != ""
}

// environmentFile reports whether name is one an environment is saved under.
// No fixture needs one, and an environment dump carries the session's
// credentials, so the file is a finding whatever it holds.
func environmentFile(name string) bool {
	b := strings.ToLower(path.Base(name))
	return strings.HasSuffix(b, ".env") || strings.HasPrefix(b, ".env.")
}

// Scan returns what data, the contents of the file name, carries that a
// committed fixture must not. An archive is scanned member by member, as the
// files it would unpack into; anything that is not text is a finding of its
// own, because a scan that cannot read a file cannot vouch for it.
func Scan(name string, data []byte) []Finding {
	if archiveKind(name) != "" {
		return scanArchive(name, data)
	}
	if environmentFile(name) {
		what := "an environment file, which no fixture needs: delete it"
		return []Finding{{name, 0, what, fingerprint(name, what, "")}}
	}
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		what := "not text, so not read: commit a fixture as text"
		return []Finding{{name, 0, what, fingerprint(name, what, "")}}
	}
	text := string(data)
	s := &scanner{path: name, digests: digests(text), seen: map[Finding]bool{}}
	units(text, func(unit string, line int) string {
		if isDocument(unit) {
			s.document(unit, lineCounter(unit, line))
		} else {
			s.text(unit, line)
		}
		return unit
	})
	return s.found
}

type scanner struct {
	path    string
	digests map[string]bool
	seen    map[Finding]bool
	found   []Finding
}

// add records a finding about value. Two findings of one shape on one line
// stay two when their values differ, so excusing one cannot excuse the other.
func (s *scanner) add(line int, what, value string) {
	f := Finding{s.path, line, what, fingerprint(s.path, what, value)}
	if !s.seen[f] {
		s.seen[f] = true
		s.found = append(s.found, f)
	}
}

// lineCounter maps an offset in doc to its line; offsets arrive in order.
func lineCounter(doc string, first int) func(int) int {
	line, last := first, 0
	return func(at int) int {
		if at > last {
			line += strings.Count(doc[last:at], "\n")
			last = at
		}
		return line
	}
}

func (s *scanner) document(doc string, lineAt func(int) int) {
	rewrite(doc, func(member, v string, at int) string {
		s.value(member, v, lineAt(at))
		return v
	}, func(member string, n, at int) bool {
		if n > 0 {
			s.add(lineAt(at), fmt.Sprintf("an account inventory: the %s list, %d entries, which scrub replaces with its count", member, n), member)
		}
		return false
	})
}

// value judges one string of a document. The deny list's key rule needs the
// name and the value in one text, and a JSON member holds them apart, so a
// non-empty value under a secret-named member is a finding of its own.
func (s *scanner) value(member, v string, line int) {
	if isDocument(v) {
		s.document(v, func(int) int { return line })
		return
	}
	if member != "" && v != "" && v != secretshape.Redacted && secretshape.SecretNamed(member) {
		s.add(line, fmt.Sprintf("a value under %q, a member the deny list names secret", member), v)
	}
	if !s.digests[v] && secretshape.Secretish(v) {
		s.add(line, "a secret shape", v)
	}
	s.shapes(v, line)
}

func (s *scanner) text(line string, n int) {
	if secretshape.Secretish(line) {
		s.add(n, "a secret shape", line)
	}
	s.shapes(line, n)
}

// shapes are the ones that name an account rather than open it.
func (s *scanner) shapes(v string, line int) {
	if m := inventoryInText.FindStringSubmatch(v); m != nil {
		s.add(line, "an account inventory: the "+m[1]+" list, inside text scrub cannot rewrite", v)
	}
	if len(emailSpans(v)) > 0 {
		s.add(line, "an email address", v)
	}
	if uuidInPath(v) {
		s.add(line, "an account-style UUID inside a path", v)
	}
	if homeIn(v) {
		s.add(line, "a home directory, or a project directory named after one", v)
	}
}

func archiveKind(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.HasSuffix(n, ".tar.gz"), strings.HasSuffix(n, ".tgz"):
		return "tgz"
	case strings.HasSuffix(n, ".tar"):
		return "tar"
	case strings.HasSuffix(n, ".zip"):
		return "zip"
	case strings.HasSuffix(n, ".gz"):
		return "gz"
	}
	return ""
}

// scanArchive scans each member as the file it unpacks into, so a capture
// cannot reach the repository by being packed first.
func scanArchive(name string, data []byte) []Finding {
	var found []Finding
	err := members(name, data, func(member string, body []byte) {
		found = append(found, Scan(name+"!"+member, body)...)
	})
	if err != nil {
		what := fmt.Sprintf("an archive that does not read to the end (%v), so what it holds is not vouched for", err)
		found = append(found, Finding{name, 0, what, fingerprint(name, what, "")})
	}
	return found
}

func members(name string, data []byte, f func(string, []byte)) error {
	kind := archiveKind(name)
	if kind == "zip" {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return err
		}
		for _, zf := range zr.File {
			if zf.FileInfo().IsDir() {
				continue
			}
			rc, err := zf.Open()
			if err != nil {
				return err
			}
			body, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return err
			}
			f(zf.Name, body)
		}
		return nil
	}
	if kind == "gz" || kind == "tgz" {
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return err
		}
		if data, err = io.ReadAll(zr); err != nil {
			return err
		}
		if kind == "gz" {
			base := path.Base(name)
			f(base[:len(base)-len(".gz")], data)
			return nil
		}
	}
	tr := tar.NewReader(bytes.NewReader(data))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			return err
		}
		f(h.Name, body)
	}
}
