package pgraph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Caps on what Load will read.
const (
	// MaxSourceFileBytes bounds one source file read for hashing and parsing.
	MaxSourceFileBytes = 8 << 20
	// MaxPackages bounds the packages one scan root may resolve to.
	MaxPackages = 50_000
)

var (
	platformName = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
	tagName      = regexp.MustCompile(`^[A-Za-z0-9_.]+$`)
)

// Config is the build configuration a scan is read under. cgo is always off:
// the graph is of pure Go, and cgo boundaries are a later slice's "unknown".
// Zero GOOS and GOARCH mean the host's.
type Config struct {
	GOOS, GOARCH string
	Tags         []string
}

// canon fills defaults and sorts and de-duplicates the tags, without
// validating.
func (c Config) canon() Config {
	if c.GOOS == "" {
		c.GOOS = runtime.GOOS
	}
	if c.GOARCH == "" {
		c.GOARCH = runtime.GOARCH
	}
	tags := append([]string(nil), c.Tags...)
	sort.Strings(tags)
	c.Tags = tags[:0]
	for i, t := range tags {
		if i == 0 || t != tags[i-1] {
			c.Tags = append(c.Tags, t)
		}
	}
	return c
}

// String is the canonical spelling stored on facts: goos/goarch/cgo=0/tags=a,b.
func (c Config) String() string {
	c = c.canon()
	return fmt.Sprintf("%s/%s/cgo=0/tags=%s", c.GOOS, c.GOARCH, strings.Join(c.Tags, ","))
}

// validate refuses anything that could be read as a flag or a path.
func (c Config) validate() error {
	if !platformName.MatchString(c.GOOS) {
		return fmt.Errorf("pgraph: bad GOOS %q", c.GOOS)
	}
	if !platformName.MatchString(c.GOARCH) {
		return fmt.Errorf("pgraph: bad GOARCH %q", c.GOARCH)
	}
	for _, t := range c.Tags {
		if !tagName.MatchString(t) {
			return fmt.Errorf("pgraph: bad build tag %q", t)
		}
	}
	return nil
}

// forced is every variable the loader pins, in output order. GOFLAGS replaces
// the caller's so a stray -mod=mod cannot make the go tool edit go.mod or
// fetch; GOWORK and GOENV keep a workspace file or a user env file from
// changing what a scan root resolves to; GOPACKAGESDRIVER keeps go/packages
// from delegating to an external program.
func forced(c Config) []string {
	c = c.canon()
	return []string{
		"GOPROXY=off", "GOTOOLCHAIN=local", "GOSUMDB=off",
		"GOFLAGS=-mod=readonly", "GOWORK=off", "GOENV=off", "GO111MODULE=on",
		"GOPACKAGESDRIVER=off", "CGO_ENABLED=0", "GOOS=" + c.GOOS, "GOARCH=" + c.GOARCH,
	}
}

// LoaderEnv returns base with the loader's pinned variables replacing any the
// caller had set: GOPROXY=off, GOTOOLCHAIN=local and GOSUMDB=off, so the go
// tool cannot reach the network to fetch a module, a toolchain or a checksum.
// Everything else passes through in order.
func LoaderEnv(base []string, c Config) []string {
	pin := forced(c)
	drop := make(map[string]bool, len(pin))
	for _, kv := range pin {
		k, _, _ := strings.Cut(kv, "=")
		drop[k] = true
	}
	out := make([]string, 0, len(base)+len(pin))
	for _, kv := range base {
		if k, _, _ := strings.Cut(kv, "="); !drop[k] {
			out = append(out, kv)
		}
	}
	return append(out, pin...)
}

// File is one ingested input, keyed by its path relative to the scan root
// (slash-separated, cleaned) and the SHA-256 of its bytes. Modification time
// plays no part.
type File struct{ Path, SHA256 string }

// Program is a loaded scan root: the packages go list ./... resolves under one
// Config, and the files that went into them. Read-only after Load.
type Program struct {
	cfg   Config
	files []File
	pkgs  []*packages.Package // sorted by PkgPath
	fset  *token.FileSet
}

func (p *Program) Config() Config { return p.cfg }
func (p *Program) Files() []File  { return append([]File(nil), p.files...) }
func (p *Program) PackagePaths() []string {
	out := make([]string, len(p.pkgs))
	for i, pk := range p.pkgs {
		out[i] = pk.PkgPath
	}
	return out
}

// Digest identifies the snapshot's inputs: the config and every file's path
// and hash. Equal digests mean equal inputs to the loader.
func (p *Program) Digest() string {
	h := sha256.New()
	io.WriteString(h, p.cfg.String()+"\n")
	for _, f := range p.files {
		io.WriteString(h, f.Path+"\x00"+f.SHA256+"\n")
	}
	return hex.EncodeToString(h.Sum(nil))
}

// LoadError reports every package the type checker or go list refused. The
// load fails closed: a partial program would read as a program with fewer
// edges.
type LoadError struct{ Messages []string }

func (e *LoadError) Error() string {
	m := e.Messages
	more := ""
	if len(m) > 5 {
		more = fmt.Sprintf(" (and %d more)", len(m)-5)
		m = m[:5]
	}
	return "pgraph: load failed: " + strings.Join(m, "; ") + more
}

// Load resolves ./... under root, a directory holding a go.mod, with the build
// configuration cfg, type-checks every package from source and hashes the
// files. The go tool runs in LoaderEnv. Test files are not part of a
// Program. Time is dominated by go list and type checking.
func Load(ctx context.Context, root string, cfg Config) (*Program, error) {
	cfg = cfg.canon()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(root)
	if err == nil {
		abs, err = filepath.EvalSymlinks(abs)
	}
	if err != nil {
		return nil, fmt.Errorf("pgraph: scan root: %w", err)
	}
	if _, err := os.Stat(filepath.Join(abs, "go.mod")); err != nil {
		return nil, fmt.Errorf("pgraph: scan root is not a Go module: %w", err)
	}
	pc := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports |
			packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo,
		Context: ctx,
		Dir:     abs,
		Env:     LoaderEnv(os.Environ(), cfg),
	}
	if len(cfg.Tags) > 0 {
		pc.BuildFlags = []string{"-tags=" + strings.Join(cfg.Tags, ",")}
	}
	loaded, err := packages.Load(pc, "./...")
	if err != nil {
		return nil, fmt.Errorf("pgraph: %w", err)
	}
	if len(loaded) > MaxPackages {
		return nil, fmt.Errorf("pgraph: %d packages, cap %d", len(loaded), MaxPackages)
	}
	var msgs []string
	var pkgs []*packages.Package
	for _, pk := range loaded {
		if len(pk.Errors) == 0 {
			pkgs = append(pkgs, pk)
			continue
		}
		for _, e := range pk.Errors {
			msgs = append(msgs, pk.PkgPath+": "+e.Error())
		}
	}
	if len(msgs) > 0 {
		sort.Strings(msgs)
		return nil, &LoadError{msgs}
	}
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].PkgPath < pkgs[j].PkgPath })
	prog := &Program{cfg: cfg, pkgs: pkgs, fset: token.NewFileSet()}
	if len(pkgs) > 0 {
		prog.fset = pkgs[0].Fset
	}
	seen := map[string]bool{}
	add := func(abspath string, optional bool) error {
		rel, err := normaliseUnder(abs, abspath)
		if err != nil {
			if optional && errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if seen[rel] {
			return nil
		}
		seen[rel] = true
		sum, err := hashFile(abspath)
		if err != nil {
			return err
		}
		prog.files = append(prog.files, File{rel, sum})
		return nil
	}
	for _, name := range []string{"go.mod", "go.sum"} {
		if err := add(filepath.Join(abs, name), name == "go.sum"); err != nil {
			return nil, err
		}
	}
	for _, pk := range pkgs {
		for _, f := range pk.CompiledGoFiles {
			if err := add(f, false); err != nil {
				return nil, err
			}
		}
	}
	sort.Slice(prog.files, func(i, j int) bool { return prog.files[i].Path < prog.files[j].Path })
	return prog, nil
}

// normalisePath returns abs as a clean slash path relative to root, refusing
// anything that resolves (through symlinks) outside it.
func normalisePath(root, abs string) (string, error) {
	r, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	return normaliseUnder(r, abs)
}

// normaliseUnder is normalisePath for a root already symlink-resolved.
func normaliseUnder(realRoot, abs string) (string, error) {
	a, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(realRoot, a)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("pgraph: %s is outside the scan root", abs)
	}
	return filepath.ToSlash(rel), nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, MaxSourceFileBytes+1))
	if err != nil {
		return "", err
	}
	if n > MaxSourceFileBytes {
		return "", fmt.Errorf("pgraph: %s is over the %d byte source cap", path, MaxSourceFileBytes)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
