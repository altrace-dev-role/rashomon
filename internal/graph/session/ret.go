// Package session is the session graph's records for one task: what was
// said (kept by reference into the transcript, never copied unless the
// bodies switch covers the project), what was delivered and whether the
// agent demonstrably acted on it, what was touched, what is owed, and the
// two surfaces that read them: the boundary brief after a compaction and
// the Stop verdict. It proves contracts-v2 sections 1 and 4.
package session

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
)

// Class is whose words a retained record holds. Project content (paths,
// symbol names, commands) is class P and is kept in clear; the person's
// words are U and the model's M, kept by reference.
type Class string

// The classes.
const (
	ClassProject Class = "P"
	ClassUser    Class = "U"
	ClassModel   Class = "M"
)

// Ref is where the words are: a transcript line counted from 1, as
// directive.LastBoundary counts, and the byte range of the words inside
// that line's extracted text.
type Ref struct {
	Path string
	Line int
	Off  int
	Len  int
}

// Ret is one retained record (contracts-v2 section 1.1): the digest and
// byte length of the words, where they are, and the words themselves only
// when Copy was allowed at write time.
type Ret struct {
	ID     string
	Class  Class
	What   string
	Origin string
	Digest string
	Bytes  int
	Ref    *Ref
	Copy   string
}

// The reasons Words gives for not delivering words.
const (
	ReasonDigestOK   = "digest ok"
	ReasonNoRef      = "no reference"
	ReasonUnreadable = "transcript not readable"
	ReasonChanged    = "transcript changed"
	ReasonCopy       = "copy"
)

// maxLine caps one transcript line read; a longer line is not read.
const maxLine = 1 << 20

// Words is the record's words and how they were had: the copy when there
// is one; else the referenced line's text, through extract, with the
// digest checked. Any failure delivers nothing and names why.
func (r Ret) Words(digest func(string) string, extract func(line []byte) (string, bool), capBytes int64) (string, string) {
	if r.Copy != "" {
		return r.Copy, ReasonCopy
	}
	if r.Ref == nil {
		return "", ReasonNoRef
	}
	line, err := readLineN(r.Ref.Path, r.Ref.Line, capBytes)
	if err != nil {
		return "", ReasonUnreadable
	}
	text, ok := extract(line)
	if !ok || r.Ref.Off < 0 || r.Ref.Off+r.Ref.Len > len(text) {
		return "", ReasonChanged
	}
	words := text[r.Ref.Off : r.Ref.Off+r.Ref.Len]
	if digest(words) != r.Digest || len(words) != r.Bytes {
		return "", ReasonChanged
	}
	return words, ReasonDigestOK
}

// errNoLine is readLineN's refusal when the file has fewer lines.
var errNoLine = errors.New("session: transcript has no such line")

// readLineN is line n of the file at path without its line feed. A file
// over capBytes is not read; a line over maxLine is not returned.
func readLineN(path string, n int, capBytes int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if fi.Size() > capBytes {
		return nil, errors.New("session: transcript over the read cap")
	}
	r := bufio.NewReaderSize(f, maxLine)
	for i := 1; ; i++ {
		line, err := r.ReadSlice('\n')
		if err == bufio.ErrBufferFull {
			return nil, errors.New("session: transcript line over the line cap")
		}
		if i == n && (err == nil || err == io.EOF) && len(line) > 0 {
			return bytes.TrimSuffix(line, []byte{'\n'}), nil
		}
		if err == io.EOF {
			return nil, errNoLine
		}
		if err != nil {
			return nil, err
		}
	}
}
