// Package hookio is the payload intake every hook entry reads stdin through.
//
// Read takes one payload with bounded growth, Intake validates and scans it
// once into the spans of the keys any consumer reads, and IntakeStream walks a
// payload that is never held. No consumer decodes the raw payload: they read
// spans, and large fields through jsonscan's Head today. Tail and Lines are
// staged for later PRs in this stack; nothing here calls them yet.
package hookio

import (
	"errors"
	"io"
	"runtime"
)

// MaxPayloadBytes bounds a buffered payload. Exactly this many bytes are
// accepted; one more is refused with ErrTooLarge after a bounded read.
const MaxPayloadBytes = 8 << 20

// ErrTooLarge reports a payload over MaxPayloadBytes. The hook's coverage
// record names it payload_too_large.
var ErrTooLarge = errors.New("hookio: payload exceeds the read limit")

// Read chunk sizes. Later chunks grow with what has been read, to an eighth
// of it, so the unused tail of the last chunk stays under an eighth of the
// payload: with the final copy that keeps Read under 2.2 times the payload
// from 40 KB up, where a fixed 64 KiB chunk would cost 2.7 times at 40 KB.
const (
	firstChunk = 4 << 10
	minChunk   = 4 << 10
	maxChunk   = 64 << 10
)

// Read reads one payload with bounded growth: a 4 KiB first chunk, then
// chunks growing to 64 KiB, then one exact-size copy. It reads at most
// MaxPayloadBytes+1 bytes, so an over-cap payload is refused after a bounded
// read, as io.ReadAll over io.LimitReader refused it, without its doubling.
func Read(r io.Reader) ([]byte, error) {
	var chunks [][]byte
	total := 0
	size := firstChunk
	for {
		if room := MaxPayloadBytes + 1 - total; size > room {
			size = room
		}
		c := make([]byte, size)
		n, err := io.ReadFull(r, c)
		total += n
		if n > 0 {
			chunks = append(chunks, c[:n])
		}
		if total > MaxPayloadBytes {
			return nil, ErrTooLarge
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return nil, err
		}
		size = min(max(total/8, minChunk), maxChunk)
	}
	if len(chunks) == 1 && len(chunks[0]) == total && cap(chunks[0]) == total {
		return chunks[0], nil
	}
	out := make([]byte, 0, total)
	for _, c := range chunks {
		out = append(out, c...)
	}
	if total >= collectAfterBytes {
		// The chunks are garbage as large as the payload. Collected now, the
		// allocations after Read can reuse their pages rather than touch new
		// ones (RES-28: hook.1MiB measured 10.09 MiB without, 9.59 with).
		chunks = nil
		runtime.GC()
	}
	return out, nil
}

// collectAfterBytes is the payload size from which Read collects its chunks
// before returning. Below it the chunks cost less than the collection.
const collectAfterBytes = 1 << 20
