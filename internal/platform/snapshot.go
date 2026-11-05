package platform

import (
	"bufio"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/regfile"
)

// The symbols sidecar pgraph writes beside the log and the digests
// (pgraph/symbols.go SymbolsFile, SymbolsHeader): its header, then
// "digests\t<sha256>" of the digests file it was written beside, then one
// row per declared symbol in id order: id, package path, file under the
// sync root, line, and the first sentence of the doc comment, tab-separated,
// file and doc possibly empty. The name and the header are the boundary, as
// LogHeader and DigestsHeader are. A reader takes it only through the walk
// file's sha256 and its own digests line, so the three files it reads are
// one sync's (walk.go).
const (
	symbolsFile = "pgraph.symbols"
	// SymbolsHeader is the one symbols format this package reads.
	SymbolsHeader = "pgraph-symbols v0"
	// MaxSymbolsBytes caps the symbols file one read takes whole (INV-10),
	// the log's cap: five fields at pgraph's field cap per row.
	MaxSymbolsBytes = MaxLogBytes
	maxSymbolsLine  = 5*maxFieldBytes + 4
)

func symbolsPath(root string) string { return filepath.Join(root, stateDir, symbolsFile) }

// ErrNotSynced is a root with no walk file, or one from a pgraph that wrote
// no symbols file: nothing to read as a Snapshot until `pgraph sync` runs.
var ErrNotSynced = errors.New("platform: no pgraph sync with a symbols file under this root; run pgraph sync")

// ErrUnvouched is a log, digests or symbols file that is not the one the
// walk file names, or a symbols file chained to other digests: another
// sync's, read as nothing (errNotVouched by its exported name).
var ErrUnvouched = errNotVouched

// The v0 predicate and tier spellings a reader of Snapshot.Facts compares
// against (decode.go).
const (
	PredDeclares   = predDeclares
	PredCalls      = predCalls
	PredEmbeds     = predEmbeds
	PredImplements = predImplements
	PredImplPtr    = predImplPtr
	PredMayCall    = predMayCall

	TierProven      = tierProven
	TierAlgorithmic = tierAlgorithmic
	TierProposed    = tierProposed
)

// Fact is one live statement of the log: subject, predicate, object, the
// tier of what made it and the build config it was read under (fact,
// exported). Commit and extractor are provenance the log keeps and a
// Snapshot does not.
type Fact struct {
	Subject, Predicate, Object, Tier, Config string
}

// Place is one declared symbol's row of the symbols file: its id as the log
// spells it, the package path the id names, the file that declares it as a
// slash path under the root ("" when pgraph could not place it), the line
// of its name, and the first sentence of its doc comment ("" with none).
type Place struct {
	ID, Pkg, File string
	Line          int
	Doc           string
}

// Snapshot is the whole of what one pgraph sync wrote under a root, read as
// one: every live Fact in subject, predicate, object, tier, config order;
// every Place in id order; and Files, each package path's files under the
// root in path order, from the digests. It is the Go producer's whole read
// (W-M1.2): the resolver that builds graph records from it reads nothing
// else of pgraph's.
type Snapshot struct {
	Facts  []Fact
	Places []Place
	Files  map[string][]string
}

// ReadSnapshot reads the sync under root whole, or refuses it whole. The
// walk file is read first; with none, or one that names no symbols file,
// it is ErrNotSynced. The log, the digests and the symbols file are then
// each read against the one walk file read (walk.go): a file that hashes
// otherwise, or a symbols file whose digests line is not the digests file's
// sha256, is ErrUnvouched, so a sync that renames between the reads is
// read as nothing. A file this package cannot read whole (another header, a
// torn or malformed line, over its cap) is its error. The decode runs in a
// worker that looks at ctx (inWorker); ctx's error is returned as soon as
// ctx is done. Any error is an empty Snapshot: never a partial answer.
func ReadSnapshot(ctx context.Context, root string) (Snapshot, error) {
	if root == "" {
		return Snapshot{}, ErrNotSynced
	}
	v, err := readVouch(root)
	if err != nil {
		return Snapshot{}, err
	}
	if !v.on || v.symbols == "" {
		return Snapshot{}, ErrNotSynced
	}
	hook := recordHook
	return inWorker(ctx, func() (Snapshot, error) { return snapshot(ctx, root, v, hook) })
}

// snapshot is ReadSnapshot's read of the three files against the walk v.
func snapshot(ctx context.Context, root string, v vouch, hook func()) (Snapshot, error) {
	live, err := readLog(ctx, LogPath(root), MaxLogBytes, v.log, hook, nil)
	if err != nil {
		return Snapshot{}, err
	}
	dg, err := readDigests(digestsPath(root))
	if err != nil {
		return Snapshot{}, err
	}
	if dg.sum != v.digests {
		return Snapshot{}, errNotVouched
	}
	places, err := readSymbols(symbolsPath(root), v.symbols, dg.sum)
	if err != nil {
		return Snapshot{}, err
	}
	facts := make([]Fact, 0, len(live))
	for f := range live {
		facts = append(facts, Fact{Subject: f.subject, Predicate: f.predicate, Object: f.object, Tier: f.tier, Config: f.config})
	}
	slices.SortFunc(facts, func(a, b Fact) int {
		return cmp.Or(cmp.Compare(a.Subject, b.Subject), cmp.Compare(a.Predicate, b.Predicate),
			cmp.Compare(a.Object, b.Object), cmp.Compare(a.Tier, b.Tier), cmp.Compare(a.Config, b.Config))
	})
	files := make(map[string][]string, len(dg.byPkg))
	for pkg, ps := range dg.byPkg {
		files[pkg] = slices.Clone(ps)
	}
	return Snapshot{Facts: facts, Places: places, Files: files}, nil
}

// readSymbols is the symbols file at p, refused whole over MaxSymbolsBytes,
// when it is no regular file, wherever parseSymbols refuses it, and when
// its sha256, taken from the same open file as it is parsed, is not want,
// the one the walk file names (errNotVouched).
func readSymbols(p, want, digestsSum string) ([]Place, error) {
	f, err := regfile.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > MaxSymbolsBytes {
		return nil, fmt.Errorf("platform: the symbols are over %d bytes", MaxSymbolsBytes)
	}
	h := sha256.New()
	places, err := parseSymbols(io.TeeReader(f, h), digestsSum)
	if err != nil {
		return nil, err
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return nil, errNotVouched
	}
	return places, nil
}

// parseSymbols reads a v0 symbols file. It refuses another header, a second
// line that is not "digests" and a sha256, a chain to other digests than
// digestsSum (errNotVouched), a line over maxSymbolsLine, a torn last line,
// a row that is not five tab-separated fields with a non-empty id and
// package and a count for its line, a field over the field cap, and a row
// out of id order, so a Place is never read from a file this package cannot
// read whole.
func parseSymbols(r io.Reader, digestsSum string) ([]Place, error) {
	br := bufio.NewReaderSize(r, maxSymbolsLine+1)
	header, err := readLine(br)
	if err != nil || string(header) != SymbolsHeader {
		return nil, errors.New("platform: the symbols' header is not " + SymbolsHeader)
	}
	chain, err := readLine(br)
	if err != nil {
		return nil, fmt.Errorf("platform: symbols line 2: %w", err)
	}
	sum, ok := strings.CutPrefix(string(chain), "digests\t")
	if !ok || !isSHA256(sum) {
		return nil, errors.New("platform: symbols line 2 is not digests and a sha256")
	}
	if sum != digestsSum {
		return nil, errNotVouched
	}
	out := []Place{}
	for n := 3; ; n++ {
		line, err := readLine(br)
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("platform: symbols line %d: %w", n, err)
		}
		f := strings.Split(string(line), "\t")
		if len(f) != 5 || f[0] == "" || f[1] == "" {
			return nil, fmt.Errorf("platform: symbols line %d is not id, package, file, line and doc", n)
		}
		for i, s := range f {
			if len(s) > maxFieldBytes {
				return nil, fmt.Errorf("platform: symbols line %d: field %d is over %d bytes", n, i+1, maxFieldBytes)
			}
		}
		ln, err := strconv.Atoi(f[3])
		if err != nil || ln < 0 {
			return nil, fmt.Errorf("platform: symbols line %d: %q is no line number", n, f[3])
		}
		if len(out) > 0 && out[len(out)-1].ID >= f[0] {
			return nil, fmt.Errorf("platform: symbols line %d: %s is out of id order", n, f[0])
		}
		out = append(out, Place{ID: f[0], Pkg: f[1], File: f[2], Line: ln, Doc: f[4]})
	}
}
