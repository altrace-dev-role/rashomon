package platform

import (
	"errors"
	"path/filepath"
	"testing"
)

// TestReadVouch_SymbolsLine (W-M1.1): a walk file from a pgraph that writes
// the symbols sidecar carries a fifth "symbols\t<sha256>" line, which the
// reader keeps; one from before it (four lines) still vouches for its pair
// with no symbols digest; a symbols line that is not a sha256, or any line
// after it, is no vouch at all.
func TestReadVouch_SymbolsLine(t *testing.T) {
	head := walkHeader + "\nwalk\t" + sumOf("walk") + "\nlog\t" + sumOf("log") + "\ndigests\t" + sumOf("digests") + "\n"
	for name, tc := range map[string]struct {
		body    string
		symbols string
		refused bool
	}{
		"old pair":     {body: head, symbols: ""},
		"with symbols": {body: head + "symbols\t" + sumOf("symbols") + "\n", symbols: sumOf("symbols")},
		"bad symbols":  {body: head + "symbols\tnot-a-sha\n", refused: true},
		"other key":    {body: head + "extra\t" + sumOf("x") + "\n", refused: true},
		"six lines":    {body: head + "symbols\t" + sumOf("symbols") + "\nmore\t" + sumOf("y") + "\n", refused: true},
	} {
		root := t.TempDir()
		put(t, filepath.Join(root, stateDir, walkFile), []byte(tc.body))
		v, err := readVouch(root)
		if tc.refused {
			if !errors.Is(err, errNotVouched) || v.on {
				t.Errorf("%s: got %+v, %v; want errNotVouched and no vouch", name, v, err)
			}
			continue
		}
		if err != nil || !v.on || v.log != sumOf("log") || v.digests != sumOf("digests") || v.symbols != tc.symbols {
			t.Errorf("%s: got %+v, %v; want on with log, digests and symbols %q", name, v, err, tc.symbols)
		}
	}
}
