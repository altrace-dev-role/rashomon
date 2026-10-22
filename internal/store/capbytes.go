package store

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultCapBytes bounds the store before eviction starts removing the oldest
// runs. EnvCapBytes overrides it.
const DefaultCapBytes = 512 << 20

// EnvCapBytes names the variable that sets the store's size cap: a plain
// decimal count of bytes, where 0 turns eviction off.
const EnvCapBytes = "RASHOMON_STORE_CAP_BYTES"

// CapSetting is the store's size cap as EnvCapBytes resolves it.
type CapSetting struct {
	// Bytes is the cap in force: the variable's value, 0 when that value
	// turns eviction off, or DefaultCapBytes when it is unset or invalid.
	Bytes int64
	// Raw is the variable's value as it was read, "" when unset.
	Raw string
	// Invalid is set when Raw is not a byte count, and DefaultCapBytes
	// applies in its place.
	Invalid bool
}

// ResolveCap reads EnvCapBytes.
func ResolveCap() CapSetting { return parseCap(os.Getenv(EnvCapBytes)) }

// CapBytes is the cap Evict is given.
//
// Two values are not a cap. Zero turns eviction off: the store then keeps
// every run, which is what a user who set it asked for. A value that is not
// a plain decimal byte count -- "2GB", "1e9", "-1" -- is invalid, and the
// default applies; it is read inside a hook, which cannot print, so status
// and watch are where it is reported (CapSetting.Problem). A negative value
// used to turn eviction off as zero does, and anything else unparseable fell
// back to the default without a word (hld-7).
func CapBytes() int64 { return ResolveCap().Bytes }

// parseCap resolves one value of EnvCapBytes. ParseUint takes no sign, no
// spaces, no underscores and no exponent, and its 63 bits are an int64's.
func parseCap(raw string) CapSetting {
	c := CapSetting{Bytes: DefaultCapBytes, Raw: raw}
	if raw == "" {
		return c
	}
	n, err := strconv.ParseUint(raw, 10, 63)
	if err != nil {
		c.Invalid = true
		return c
	}
	c.Bytes = int64(n)
	return c
}

// Off reports whether eviction is off.
func (c CapSetting) Off() bool { return c.Bytes == 0 }

// Problem says why the variable's value was not taken, or is "" when it was
// taken or unset.
func (c CapSetting) Problem() string {
	if !c.Invalid {
		return ""
	}
	return fmt.Sprintf("%s=%q is not a byte count (a plain decimal number; 0 turns eviction off), so the default cap of %s applies",
		EnvCapBytes, c.Raw, FormatBytes(DefaultCapBytes))
}

// RecordedCap is the cap a hook resolved and wrote with RecordResolvedCap:
// the environment eviction actually ran in, as opposed to whatever shell
// a later status call happens to run in (hld-7).
type RecordedCap struct {
	Bytes   int64
	Problem string
}

// RecordResolvedCap writes c, as resolved inside a hook invocation, to the
// store root. It is a note, like the lock-wait one: best-effort, not synced,
// and a write that fails or loses a race with another invocation leaves the
// previous note standing rather than a torn one -- the file is replaced
// whole, by writeSmall, so the race is between two complete versions, never
// half of one and half of the other.
func (s *Store) RecordResolvedCap(c CapSetting) error {
	if err := s.opened(); err != nil {
		return err
	}
	content := []byte(strconv.FormatInt(c.Bytes, 10) + "\n" + c.Problem() + "\n")
	return writeSmall(filepath.Join(s.root, FileResolvedCap), content, syncFull)
}

// ResolvedCap reads the cap the last hook invocation recorded. ok is false
// when no hook has recorded one yet (a store from before this field existed,
// or one no hook invocation has touched), or the note does not parse, which
// reads as none rather than as a wrong value.
func (s *Store) ResolvedCap() (c RecordedCap, ok bool, err error) {
	b, rerr := os.ReadFile(filepath.Join(s.root, FileResolvedCap))
	if errors.Is(rerr, fs.ErrNotExist) {
		return RecordedCap{}, false, nil
	}
	if rerr != nil {
		return RecordedCap{}, false, rerr
	}
	line, rest, _ := strings.Cut(string(b), "\n")
	n, perr := strconv.ParseInt(strings.TrimSpace(line), 10, 64)
	if perr != nil {
		return RecordedCap{}, false, nil
	}
	return RecordedCap{Bytes: n, Problem: strings.TrimSpace(rest)}, true, nil
}

// RunsBytes is the size of every run, which is what Evict holds under the
// cap, and how many runs it had to leave out of that total because their
// directory could not be listed, as Evict leaves them out of its plan. The
// total is of the runs it could list even when unlisted is over zero: a
// caller that wants the figure anyway, with a word about what it is missing,
// does not have to throw away a correct partial sum because one run's size
// could not be read.
func (s *Store) RunsBytes() (bytes int64, unlisted int, err error) {
	plan, err := evictCandidates(filepath.Join(s.root, DirRuns))
	if err != nil {
		return 0, 0, err
	}
	return plan.total, len(plan.unlisted), errors.Join(plan.unlisted...)
}

// FormatBytes is n in the largest binary unit it reaches, to one decimal
// place, with a whole number printed without one: "512 MiB", "1.5 KiB".
//
// The unit is picked before rounding, so a value just under a boundary --
// 1048575 B, one byte short of 1 MiB -- used to round to "1024.0" and print
// with the smaller unit's suffix still attached ("1024 KiB"). Rounding is
// checked against the unit after formatting, and bumped up once more when
// it reaches it and a larger unit exists.
func FormatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}
	v, suffix := float64(n), "B"
	i := 0
	for i < len(units) && v >= unit {
		v, suffix = v/unit, units[i]
		i++
	}
	if rounded := math.Round(v*10) / 10; rounded >= unit && i < len(units) {
		v, suffix = v/unit, units[i]
	}
	text := strconv.FormatFloat(v, 'f', 1, 64)
	if len(text) > 2 && text[len(text)-2:] == ".0" {
		text = text[:len(text)-2]
	}
	return text + " " + suffix
}
