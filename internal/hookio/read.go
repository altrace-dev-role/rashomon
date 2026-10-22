// Package hookio is the payload intake every hook entry reads stdin through.
//
// Read takes one payload with bounded growth. Intake validates it with
// json.Valid and then walks its top level once, recording the spans of the
// keys any consumer reads; a consumer that reads inside tool_input or
// tool_response walks that one object again for the keys it wants, and
// canonical still decodes tool_input for its digest. IntakeStream validates
// and walks a payload in one pass and never holds it. No consumer decodes the
// raw payload: they read spans, and large fields through jsonscan's Head
// today. Tail and Lines are staged for later PRs in this stack; nothing here
// calls them yet.
package hookio

import (
	"bytes"
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
//
// The refusal is Intake's: a *TooLargeError naming the session id and cwd
// the payload's head carries, so a caller that reads first and hands the
// error on -- the host guard's replay -- still lets the refusal be recorded
// in the run the payload names rather than in the unattributed one.
func Read(r io.Reader) ([]byte, error) {
	out, head, err := read(r)
	if errors.Is(err, ErrTooLarge) {
		return nil, refused(head)
	}
	return out, err
}

// read is Read that also returns, with ErrTooLarge, the payload's first
// chunk: the head Intake attributes a refusal from.
func read(r io.Reader) (out, head []byte, err error) {
	// A *bytes.Reader already knows its own remaining length exactly --
	// Len() -- so the growth loop below, built for a source of unknown
	// length (a pipe), is pure overhead on one: at least one extra
	// allocation (the final out := make(...) copy, hit by everything under
	// firstChunk too, not just multi-chunk reads) and, above firstChunk,
	// several. cmd/rashomon's host-guard stand-down (internal/hostguard,
	// Technical spec 6.2.3) reads stdin once through this same Read to
	// decide whether to proceed, then hands a *bytes.Reader over the exact
	// same already-read bytes to the entry's normal Capture/RunProbe call,
	// which reads it through Read a second time -- doubling this
	// function's own cost on every hook, post and probe invocation whether
	// or not it stands down. This fast path is what keeps that second read
	// from being the one that is actually expensive: BenchmarkRunHook/
	// bash_short measured 68,520 B/op before it, 14.5% over its 53,523
	// budget row (the gate's own tolerance is 10%), and 55,895 B/op after
	// -- still over the row itself (that bytes.Reader is still a second
	// read of the same bytes, just a cheap one now), but inside tolerance.
	// internal/hostguard's own EvaluateRaw closes the rest of the gap for
	// a payload this does not: see its doc for why building the Payload
	// straight from already-read bytes, rather than through this function
	// a second time, removes the second read this comment describes
	// entirely on the host-guard decision's own side.
	//
	// ErrTooLarge is still exact: Len() already is the input's whole
	// remaining size, so comparing it to MaxPayloadBytes before allocating
	// the payload refuses an over-cap *bytes.Reader for the cost of its
	// first chunk, the head Intake attributes the refusal from, rather than
	// after the bounded read the general path below needs to find the same
	// answer from a source that cannot report its length upfront.
	if br, ok := r.(*bytes.Reader); ok {
		n := br.Len()
		if n > MaxPayloadBytes {
			head = make([]byte, firstChunk)
			if _, err := io.ReadFull(br, head); err != nil {
				return nil, nil, err
			}
			return nil, head, ErrTooLarge
		}
		out = make([]byte, n)
		if _, err := io.ReadFull(br, out); err != nil {
			return nil, nil, err
		}
		return out, nil, nil
	}

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
			return nil, chunks[0], ErrTooLarge
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		size = min(max(total/8, minChunk), maxChunk)
	}
	if len(chunks) == 1 && len(chunks[0]) == total && cap(chunks[0]) == total {
		return chunks[0], nil, nil
	}
	out = make([]byte, 0, total)
	for _, c := range chunks {
		out = append(out, c...)
	}
	if total >= collectAfterBytes {
		// The chunks are garbage as large as the payload. Collected now, the
		// allocations after Read can reuse their pages rather than touch new
		// ones (RES-28: hook.1MiB measured 10.09 MiB without, 9.59 with,
		// against a 9.90 line). The collection costs about 0.1 ms at 1 and
		// 4 MiB and is lost in the read's own noise at 8 MiB (BenchmarkRead,
		// darwin/arm64), against the hook rows' p50 of 10 to 15 ms.
		chunks = nil
		runtime.GC()
	}
	return out, nil, nil
}

// collectAfterBytes is the payload size from which Read collects its chunks
// before returning. Below it the chunks cost less than the collection.
const collectAfterBytes = 1 << 20
