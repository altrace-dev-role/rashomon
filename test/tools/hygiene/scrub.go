package hygiene

import (
	"bytes"
	"errors"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/secretshape"
)

// Scrubber rewrites captured payloads so they can be committed. One Scrubber
// holds the placeholder assignments, so every file scrubbed through it gives
// a UUID or an address the same placeholder, and a join across files -- a
// session id in a hook payload and in the name of the transcript beside it --
// survives.
type Scrubber struct {
	home   string
	uuids  map[string]string
	emails map[string]string
}

// NewScrubber returns a Scrubber that also replaces home, the scrubbing
// machine's own home directory, wherever it is a path prefix: a home outside
// /home, /Users and /root has no shape the patterns know.
func NewScrubber(home string) *Scrubber {
	return &Scrubber{home: home, uuids: map[string]string{}, emails: map[string]string{}}
}

// Scrub returns data, the contents of the file name, rewritten so it carries
// no secret shape, no email address, no UUID, no home, temp or scratchpad path
// and no account inventory, with every other byte as it was. A digest the
// file names one (see namedDigest) is kept, as the scan keeps it. An archive
// or a binary file is refused: unpack it, scrub each file, and pack it again.
//
// What it cannot do is read: a name, a client or a project written into a
// prompt or a command is content, not a shape, and survives. The output is
// for a person to read before it is committed, not instead.
func (z *Scrubber) Scrub(name string, data []byte) ([]byte, error) {
	if archiveKind(name) != "" {
		return nil, errors.New("an archive: unpack it, scrub each file, and pack it again")
	}
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return nil, errors.New("not text")
	}
	text := string(data)
	named := digests(text)
	return []byte(units(text, func(unit string, _ int) string {
		if isDocument(unit) {
			return z.document(unit, named)
		}
		return z.text(unit)
	})), nil
}

func (z *Scrubber) document(doc string, named map[string]bool) string {
	return rewrite(doc, func(member, v string, _ int) string {
		return z.value(member, v, named)
	}, func(string, int, int) bool { return true })
}

func (z *Scrubber) value(member, v string, named map[string]bool) string {
	switch {
	case isDocument(v):
		return z.document(v, named)
	case member != "" && v != "" && secretshape.SecretNamed(member):
		return secretshape.Redacted
	case named[v]:
		return v
	}
	return z.text(v)
}

// text rewrites one string or line. Secrets go first, a whole token at a
// time, so a credential inside a path goes with the path; then the paths,
// the scratchpad before the temp root that holds it and the home rules after
// both; then the ids, last, so the ones a path held went with it.
func (z *Scrubber) text(s string) string {
	s, _ = secretshape.Redact(s)
	s = scratchpadRE.ReplaceAllLiteralString(s, "$SCRATCHPAD")
	s = tempRE.ReplaceAllString(s, "${1}$$TMPDIR")
	s = replaceDir(s, z.home, "~")
	s = homeRE.ReplaceAllString(s, "${1}~")
	// A match takes the separator after it, which the next one may need
	// before it, so run until nothing changes; "-project" never matches.
	for prev := ""; prev != s; {
		prev, s = s, slugRE.ReplaceAllString(s, "${1}-project${3}")
	}
	s = assign(s, accountUUIDs(s), z.uuids, placeholderUUID+"4000-8000-%012x")
	return assign(s, emailSpans(s), z.emails, "user-%d@example.invalid")
}
