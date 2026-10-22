package slot

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Path is a project-relative path (archive 40.3.8; tech 4.2.7): segments
// `^[A-Za-z0-9._@+-]{1,128}$` joined by '/', at most 256 bytes, no ".." and
// no leading '-'; it must also pass the secret deny list, and no segment of
// 20 or more characters may be secret-shaped by the entropy test, except a
// digest under the directory that names its algorithm (isDigest), which is
// kept. A "." segment is refused as well: a canonical
// relative path never holds one, and VERIFY's path slot refuses it. A
// repository can name a path, so it renders inside a quote. The secret
// checks fail with ErrSecretShaped, which PathList uses to leave such a path
// out rather than refuse the note.
type Path string

var pathSegmentRE = &lazyRegexp{pattern: `^[A-Za-z0-9._@+-]{1,128}$`}

const maxPathBytes = 256

func (p Path) Validate() error {
	if err := p.checkShape(); err != nil {
		return err
	}
	return p.checkSecret()
}

func (p Path) Text() string         { return quoted(string(p)) }
func (p Path) bind() (Value, error) { return checked(p, p.Validate()) }

// checkShape applies every Path rule but the secret ones.
func (p Path) checkShape() error {
	s := string(p)
	if s == "" || len(s) > maxPathBytes {
		return fmt.Errorf("slot: path is empty or over %d bytes", maxPathBytes)
	}
	if strings.HasPrefix(s, "/") {
		return errors.New("slot: path is not relative")
	}
	for _, seg := range strings.Split(s, "/") {
		if seg == "." || seg == ".." {
			return errors.New("slot: path has a '.' or '..' segment")
		}
		if strings.HasPrefix(seg, "-") {
			return errors.New("slot: path has a segment with a leading '-'")
		}
		if err := charset("path segment", seg, pathSegmentRE); err != nil {
			return err
		}
	}
	return nil
}

// checkSecret applies the deny list to the whole path and the entropy test
// to each long segment.
func (p Path) checkSecret() error {
	s := string(p)
	if err := denied("path", s); err != nil {
		return err
	}
	parent := ""
	for _, seg := range strings.Split(s, "/") {
		if len(seg) >= entropyMinRun && !isDigest(parent, seg) && highEntropy(seg) {
			return fmt.Errorf("%w: a path segment fails the entropy test", ErrSecretShaped)
		}
		parent = seg
	}
	return nil
}

// isDigest reports seg as the digest its parent directory names: exactly
// 40 lowercase hex characters under "sha1", or 64 under "sha256", the
// layout content-addressed stores use (an OCI image's blobs/sha256/). A
// bare 40- or 64-hex segment anywhere else is tested like any other: a
// legacy GitHub token and a Datadog application key are 40 lowercase hex,
// a Linode token and a hex-encoded 32-byte key 64, and only the location
// tells a digest from them.
func isDigest(parent, seg string) bool {
	n := 0
	switch parent {
	case "sha1":
		n = 40
	case "sha256":
		n = 64
	}
	if n == 0 || len(seg) != n {
		return false
	}
	for i := 0; i < len(seg); i++ {
		if c := seg[i]; !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// PathList is the paths a note lists (tech 4.2.7): at most three render,
// each inside its own quote, then "and {k} more", with {k} counted from the
// record -- the writes in the span the list does not name, plus the listed
// paths that were left out or did not fit -- so a cut list never reads as
// complete. Build one with NewPathList; Bind returns it as a message holds
// it.
type PathList struct {
	paths []Path
	more  int
}

// maxListedPaths is how many paths a note renders before "and {k} more".
const maxListedPaths = 3

// ErrNoPathsLeft is returned by Bind when every listed path was left out:
// the pattern then uses its card's no-paths form, or the note is refused.
var ErrNoPathsLeft = errors.New("slot: no path left to list")

// NewPathList lists paths, with unlisted the writes the record shows in the
// span beyond them (VERIFY's PathsOmitted beside its Paths, archive 40.4).
func NewPathList(paths []Path, unlisted int) PathList {
	return PathList{paths: slices.Clone(paths), more: unlisted}
}

// Paths returns a copy of the listed paths.
func (l PathList) Paths() []Path { return slices.Clone(l.paths) }

// More returns the count "and {k} more" starts from: the unlisted writes,
// and once bound, the paths left out or cut as well.
func (l PathList) More() int { return l.more }

// Validate checks the list as it is: at least one path, every path passing
// every Path check, a non-negative count.
func (l PathList) Validate() error {
	if l.more < 0 {
		return errors.New("slot: path list has a negative count")
	}
	if len(l.paths) == 0 {
		return ErrNoPathsLeft
	}
	for _, p := range l.paths {
		if err := p.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (l PathList) Text() string {
	shown, more := l.paths, l.more
	if len(shown) > maxListedPaths {
		more += len(shown) - maxListedPaths
		shown = shown[:maxListedPaths]
	}
	parts := make([]string, len(shown))
	for i, p := range shown {
		parts[i] = p.Text()
	}
	s := strings.Join(parts, ", ")
	if more > 0 {
		s += " and " + strconv.Itoa(more) + " more"
	}
	return s
}

// bind leaves out every path that fails the deny list or the entropy test,
// counting it into "and {k} more"; a path failing any other check refuses
// the list, and so does a list with no path left (tech 4.2.7: "none left:
// the card's no-paths form, else `render_refused`").
func (l PathList) bind() (Value, error) {
	if l.more < 0 {
		return nil, errors.New("slot: path list has a negative count")
	}
	kept := make([]Path, 0, len(l.paths))
	more := l.more
	for _, p := range l.paths {
		err := p.Validate()
		switch {
		case errors.Is(err, ErrSecretShaped):
			more++
		case err != nil:
			return nil, err
		default:
			kept = append(kept, p)
		}
	}
	if len(kept) == 0 {
		return nil, ErrNoPathsLeft
	}
	if len(kept) > maxListedPaths {
		more += len(kept) - maxListedPaths
		kept = kept[:maxListedPaths]
	}
	return PathList{paths: kept, more: more}, nil
}
