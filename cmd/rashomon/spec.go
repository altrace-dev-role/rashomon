package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/selfguard"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// specUsage is the usage screen's spec entries, joined as learnUsage is.
const specUsage = `  rashomon spec trust [PATH]     show this project's repository spec (or the
                               one at PATH), its digest and what it declares
                               on the terminal; it counts as trusted once the
                               digest's first 8 characters are typed back
                               there, until the file is edited
  rashomon spec untrust [PATH]   take that trust back

`

// specTerminal opens the terminal the spec is shown and the answer typed on;
// a test replaces it.
var specTerminal = openTerminal

// cmdSpec is `rashomon spec trust|untrust [PATH]` (card SPEC-TRUST1, PRD
// PF-SP2), the verb spec.Trusted's comment and the consent screen name. Its
// bindings live under the store root, where discovery reads them.
func cmdSpec(args []string, stdout io.Writer) error {
	if len(args) > 0 && args[0] != "trust" && args[0] != "untrust" {
		return fmt.Errorf("unknown argument %q", args[0])
	}
	if len(args) == 0 || len(args) > 2 {
		return errors.New("spec takes trust or untrust, and at most one PATH")
	}
	path, err := specPath(args[1:])
	if err != nil {
		return err
	}
	st, err := openStoreForRead()
	if errors.Is(err, store.ErrNoStore) && args[0] == "untrust" {
		_, err = fmt.Fprintf(stdout, "%s was not trusted\n", path)
		return err
	}
	if errors.Is(err, store.ErrNoStore) {
		return errors.New("nothing has been recorded here, so nothing reads a trust binding; run rashomon watch first")
	}
	if err != nil {
		return err
	}
	b, err := spec.ReadBindings(spec.OSFS{}, st.Root())
	if errors.Is(err, spec.ErrBindingsUnsigned) {
		// Something other than this verb wrote the file (D173): it binds
		// nothing, which discovery already reads it as, and a trust
		// replaces it.
		if _, err := fmt.Fprintf(stdout, "%s does not carry this store's MAC, so it trusts nothing: something other than rashomon spec trust wrote it\n", spec.TrustFile); err != nil {
			return err
		}
		b, err = &spec.Bindings{}, nil
	}
	if err != nil {
		return err
	}
	if args[0] == "trust" {
		return trustSpec(b, st.Root(), path, stdout)
	}
	if !b.Unbind(hook.SpecProject(path), path) {
		_, err = fmt.Fprintf(stdout, "%s was not trusted\n", path)
		return err
	}
	if err := hook.WriteSpecBindings(st.Root(), b); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "untrusted %s: what it declares flags to you only\n", path)
	return err
}

// specPath is the repository spec the verb names: PATH, which must be
// <dir>/.claude/rashomon/spec.json, the one name discovery reads, with dir
// resolved as discovery resolves a cwd; with none, the nearest that exists of
// the candidates discovery walks from the working directory.
func specPath(args []string) (string, error) {
	if len(args) == 1 {
		p, err := filepath.Abs(args[0])
		if err != nil {
			return "", err
		}
		dir := filepath.Dir(filepath.Dir(filepath.Dir(p)))
		if filepath.Join(dir, ".claude", "rashomon", "spec.json") != p {
			return "", fmt.Errorf("%s is not a repository spec: discovery reads only <dir>/.claude/rashomon/spec.json", args[0])
		}
		if r, err := filepath.EvalSymlinks(dir); err == nil {
			dir = r
		}
		return filepath.Join(dir, ".claude", "rashomon", "spec.json"), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	locs := hook.SpecLocations(cwd, os.Getenv("CLAUDE_PROJECT_DIR"))
	if len(locs) == 0 {
		// A plain terminal with no .git above cwd and no CLAUDE_PROJECT_DIR:
		// Claude Code, started here, sets CLAUDE_PROJECT_DIR to this
		// directory, so the spec the hook would read is the one at cwd.
		locs = hook.SpecLocations(cwd, cwd)
	}
	for _, p := range locs {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("no repository spec applies here; name its path")
}

// trustSpec shows the spec at path, its digest and what it declares on the
// terminal, never stdout or stdin, and binds it only when the digest's first
// 8 characters are typed back there (PRD PF-SP2; design spec 5.3): an agent's
// shell has no terminal, and under an agent signal the verb does not ask
// (tech spec 7.2.4, Trust grant). What the repository wrote is quoted, the
// spec's path and project among it (their directory names are the
// repository's), so its bytes never reach the terminal raw.
func trustSpec(b *spec.Bindings, root, path string, stdout io.Writer) error {
	if s := selfguard.AgentSignal(os.Getenv); s != "" {
		return fmt.Errorf("%s is set, so this is an agent session; nothing was written. Trust a spec in your own terminal.", s)
	}
	data, err := spec.OSFS{}.ReadFile(path)
	if err != nil {
		return err
	}
	f, digest, err := spec.Parse(data)
	if err != nil {
		return fmt.Errorf("%s: %w; nothing was trusted", path, err)
	}
	in, out, closeTTY, err := specTerminal()
	if err != nil {
		return errors.New("there is no terminal to ask on, so nothing was trusted; a person trusts a spec at a terminal")
	}
	defer closeTTY()
	project, typed := hook.SpecProject(path), strings.TrimPrefix(digest, spec.DigestPrefix)[:8]
	fmt.Fprintf(out, "%q in %q\n  digest %s  now untrusted: flags to you only\n  once trusted, these count:\n", path, project, digest)
	for _, g := range f.Gates {
		fmt.Fprintf(out, "    gate  %q  %q\n", g.ID, g.Argv)
	}
	for _, argv := range f.CICommands {
		fmt.Fprintf(out, "    ci    %q\n", argv)
	}
	for _, r := range f.Rules {
		fmt.Fprintf(out, "    rule  %q\n", r.ID)
	}
	for _, x := range f.Facts {
		fmt.Fprintf(out, "    fact  %q\n", x.ID)
	}
	fmt.Fprint(out, "type the first 8 characters of the digest to trust it: ")
	line, _ := bufio.NewReader(in).ReadString('\n')
	if strings.TrimSpace(line) != typed {
		return errors.New("the characters typed did not match the digest, so nothing was trusted")
	}
	b.Bind(spec.Binding{ProjectRoot: project, SpecPath: path, SpecDigest: digest, BoundAtMS: time.Now().UnixMilli()})
	if err := hook.WriteSpecBindings(root, b); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "trusted %s at digest %s; an edit to the file untrusts it\n  undo: rashomon spec untrust %s\n", path, typed, shellWord(path))
	return err
}

// shellWord is s as one shell word: bare when it holds only safe characters,
// else single-quoted, so the undo line pastes back as the path it names.
func shellWord(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/._-+:@%") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
