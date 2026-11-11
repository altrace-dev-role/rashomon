package platform

import (
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/regfile"
)

// The walk file pgraph writes last after each sync (pgraph/cmd/pgraph
// hook.go: walkFile, walkHeader, encodeWalk): its header, the walk's digest,
// then "log\t<sha256>" and "digests\t<sha256>" of the pair that sync wrote,
// since the symbols sidecar also "symbols\t<sha256>" (absent from a walk file
// an older pgraph wrote, which still vouches for its pair), or "refused"
// where the sync failed. It is the one record that says a log and a digests
// file are one sync's: the plugin runs the sync in the background at the
// Stop (PW1b-ON-230 1b), so a Stop can read between the sync's renames, and
// a log from one sync read with the digests of another would vouch for
// facts the files no longer hold.
const (
	walkFile     = "pgraph.walk"
	walkHeader   = "pgraph-walk v0"
	maxWalkBytes = 4 << 10
)

// errNotVouched is a log or digests file whose sha256 is not the one the
// walk file names, or a walk file that names no pair.
var errNotVouched = errors.New("platform: the walk file does not vouch for this log and digests")

// vouch is what the walk file says of the pair: on false, there is no walk
// file, which a pgraph from before it writes, and nothing is checked.
// symbols is "" for a walk file from before the symbols sidecar: the pair
// is vouched for and the symbols file, if any, is not.
type vouch struct {
	on                    bool
	log, digests, symbols string
}

// readVouch reads root's walk file. A walk file that is there and names no
// pair (refused, another header, malformed, over its cap, no regular file)
// is errNotVouched: its rows read unknown, never a pair taken on trust.
func readVouch(root string) (vouch, error) {
	f, err := regfile.Open(filepath.Join(root, stateDir, walkFile))
	if errors.Is(err, fs.ErrNotExist) {
		return vouch{}, nil
	}
	if err != nil {
		return vouch{}, errNotVouched
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxWalkBytes+1))
	if err != nil || len(b) > maxWalkBytes {
		return vouch{}, errNotVouched
	}
	lines := strings.Split(string(b), "\n")
	if (len(lines) != 5 && len(lines) != 6) || lines[0] != walkHeader || !strings.HasPrefix(lines[1], "walk\t") || lines[len(lines)-1] != "" {
		return vouch{}, errNotVouched
	}
	l, lok := strings.CutPrefix(lines[2], "log\t")
	d, dok := strings.CutPrefix(lines[3], "digests\t")
	if !lok || !dok || !isSHA256(l) || !isSHA256(d) {
		return vouch{}, errNotVouched
	}
	v := vouch{on: true, log: l, digests: d}
	if len(lines) == 6 {
		s, sok := strings.CutPrefix(lines[4], "symbols\t")
		if !sok || !isSHA256(s) {
			return vouch{}, errNotVouched
		}
		v.symbols = s
	}
	return v, nil
}
