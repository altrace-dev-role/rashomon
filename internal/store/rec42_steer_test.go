package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// testSteerLine stands in for steer.Line, which this package cannot import
// (steer is the caller): the keys AppendSteer's tests read, and a CapText
// that withholds model_text the way steer.Line's does.
type testSteerLine struct {
	Type              string          `json:"type"`
	ID                string          `json:"id"`
	ModelText         string          `json:"model_text"`
	ModelTextWithheld *string         `json:"model_text_withheld"`
	Slots             json.RawMessage `json:"slots,omitempty"`
}

func (l *testSteerLine) CapText() bool {
	var withheld bool
	l.ModelText, withheld = CapSteerText(l.ModelText, &l.ModelTextWithheld)
	return withheld
}

func steerLines(ids ...string) []SteerLine {
	lines := make([]SteerLine, 0, len(ids))
	for _, id := range ids {
		lines = append(lines, &testSteerLine{Type: "steer", ID: id, ModelText: "rashomon record: " + id})
	}
	return lines
}

// fixed is a build that returns lines and reads nothing.
func fixed(lines []SteerLine) func(io.ReaderAt) ([]SteerLine, error) {
	return func(io.ReaderAt) ([]SteerLine, error) { return lines, nil }
}

// steerIDs reads the run's steer log back and returns each line's id, and
// how many lines did not read.
func steerIDs(t *testing.T, st *Store, session string) ([]string, int) {
	t.Helper()
	lines, skipped, err := st.ReadSteer(session)
	if err != nil {
		t.Fatalf("ReadSteer: %v", err)
	}
	ids := make([]string, 0, len(lines))
	for _, line := range lines {
		var l testSteerLine
		if err := json.Unmarshal(line, &l); err != nil {
			t.Fatalf("ReadSteer returned a line that does not parse: %q", line)
		}
		ids = append(ids, l.ID)
	}
	return ids, skipped
}

// lockHeld reports whether some handle holds path's lock: a second
// descriptor's one try is refused.
func lockHeld(t *testing.T, path string) bool {
	t.Helper()
	f, err := os.OpenFile(path, appendFlags, fileMode)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	unlock, _, err := lockFileWait(f, 0)
	if err == nil {
		unlock()
		return false
	}
	if !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("probing the lock of %s: %v", path, err)
	}
	return true
}

// standInEmit is the router's step 10 and 11 as far as this package can
// show them (Technical spec 4.2.4): append every line, then the one full
// sync, then stdout, and nothing on stdout when either fails. The router is
// a later change; REC-42's binary-level test drives the real one.
func standInEmit(st *Store, session string, lines []SteerLine, stdout io.Writer) {
	if err := st.AppendSteer(session, fixed(lines)); err != nil {
		return
	}
	if err := st.Flush(); err != nil {
		return
	}
	fmt.Fprint(stdout, `{"systemMessage":"rashomon record"}`)
}

// durableCheck is the stand-in's stdout. Each Write checks, at the moment
// of printing, that the steer lines it is to print are in the steer log and
// that nothing the log was appended with is still owed a sync.
type durableCheck struct {
	t       *testing.T
	st      *Store
	session string
	want    []string
	writes  int
}

func (d *durableCheck) Write(p []byte) (int, error) {
	d.t.Helper()
	d.writes++
	if d.st.dirty != "" {
		d.t.Errorf("stdout written while %s was still owed its full sync", filepath.Base(d.st.dirty))
	}
	ids, _ := steerIDs(d.t, d.st, d.session)
	if strings.Join(ids, ",") != strings.Join(d.want, ",") {
		d.t.Errorf("stdout written with the steer log holding %v, want %v", ids, d.want)
	}
	return len(p), nil
}

// TestREC42_SteerRecordBeforeEmit: a steer line is in steer.ndjson, put
// behind its barrier inside the lock and made durable by the one full sync,
// before anything reaches stdout; an append that fails prints nothing
// (Technical spec 1.2.9, 4.2.4 steps 10 and 11).
func TestREC42_SteerRecordBeforeEmit(t *testing.T) {
	const session = "sess-steer"

	t.Run("appended, ordered inside the lock, flushed, then printed", func(t *testing.T) {
		st, err := Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		var barriers []string
		st.barrier = func(f *os.File) (bool, error) {
			if !lockHeld(t, f.Name()) {
				t.Errorf("the barrier on %s ran outside its lock", filepath.Base(f.Name()))
			}
			barriers = append(barriers, filepath.Base(f.Name()))
			return false, nil
		}
		st.DeferDurability()
		out := &durableCheck{t: t, st: st, session: session, want: []string{"a", "b"}}
		standInEmit(st, session, steerLines("a", "b"), out)
		if out.writes != 1 {
			t.Fatalf("the stand-in printed %d times, want once", out.writes)
		}
		if strings.Join(barriers, ",") != FileSteer {
			t.Errorf("barriers taken: %v, want one, on %s, for both lines", barriers, FileSteer)
		}
		if st.dirty != "" {
			t.Errorf("after the flush %q is still owed a sync", st.dirty)
		}
	})

	t.Run("the append leaves its full sync to the flush", func(t *testing.T) {
		st, err := Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		st.barrier = orderingBarrier
		st.DeferDurability()
		if err := st.AppendSteer(session, fixed(steerLines("a"))); err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(st.RunDir(session), FileSteer); st.dirty != want {
			t.Fatalf("after a deferred steer append the flush would cover %q, want %q", st.dirty, want)
		}
	})

	t.Run("a held lock prints nothing and writes nothing", func(t *testing.T) {
		st, err := Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		st.barrier = orderingBarrier
		st.DeferDurability()
		path := filepath.Join(st.RunDir(session), FileSteer)
		if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
			t.Fatal(err)
		}
		holder := openForAppend(t, path)
		unlock, _, err := lockFileWait(holder, lockBudget)
		if err != nil {
			t.Fatal(err)
		}
		out := &durableCheck{t: t, st: st, session: session}
		standInEmit(st, session, steerLines("a"), out)
		unlock()
		if out.writes != 0 {
			t.Errorf("the stand-in printed after its steer append gave up")
		}
		if ids, skipped := steerIDs(t, st, session); len(ids) != 0 || skipped != 0 {
			t.Errorf("a steer append that gave up on its lock left %v (%d unreadable) in the log", ids, skipped)
		}
	})
}

// TestREC42b_SteerLockGivesUpAtItsBudget: the steer lock waits 150 ms, not
// the records' two seconds, and inside an invocation's shared deadline
// (LimitLockWaits) no longer than the time left (Technical spec 4.2.4 step
// 10, 4.2.5 "Lock timeout").
func TestREC42b_SteerLockGivesUpAtItsBudget(t *testing.T) {
	const session = "sess-steer-held"
	for _, tc := range []struct {
		name     string
		limit    time.Duration
		min, max time.Duration
	}{
		{"its own budget", 0, steerBudget, time.Second},
		{"the invocation's deadline", 20 * time.Millisecond, 0, steerBudget},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(st.RunDir(session), FileSteer)
			if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
				t.Fatal(err)
			}
			holder := openForAppend(t, path)
			unlock, _, err := lockFileWait(holder, lockBudget)
			if err != nil {
				t.Fatal(err)
			}
			defer unlock()
			if tc.limit > 0 {
				st.LimitLockWaits(tc.limit)
			}
			start := time.Now()
			err = st.AppendSteer(session, fixed(steerLines("a")))
			waited := time.Since(start)
			if !errors.Is(err, ErrLockTimeout) {
				t.Fatalf("AppendSteer under a held lock returned %v, want ErrLockTimeout", err)
			}
			if waited < tc.min || waited >= tc.max {
				t.Errorf("AppendSteer gave up after %v, want at least %v and under %v", waited, tc.min, tc.max)
			}
		})
	}
	if steerBudget != 150*time.Millisecond {
		t.Errorf("steerBudget is %v, want the 150 ms Technical spec 4.2.4 sets", steerBudget)
	}
}

// TestREC42c_EveryLineUnderOneHold: the lines of one append are decided and
// written under one hold of the steer lock -- build runs with the lock held
// and reads the log through the locked handle -- and they land together or
// not at all: a build that fails, or a line over MaxRecordBytes, writes none
// of them, and an append with no lines writes nothing and owes no sync.
func TestREC42c_EveryLineUnderOneHold(t *testing.T) {
	const session = "sess-steer-hold"

	t.Run("build runs under the lock and reads through its handle", func(t *testing.T) {
		st, err := Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if err := st.AppendSteer(session, fixed(steerLines("a"))); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(st.RunDir(session), FileSteer)
		err = st.AppendSteer(session, func(locked io.ReaderAt) ([]SteerLine, error) {
			if !lockHeld(t, path) {
				t.Error("build ran without the steer lock held")
			}
			if locked == nil {
				t.Fatal("build was given no handle to read the log through")
			}
			b, err := io.ReadAll(io.NewSectionReader(locked, 0, 1<<20))
			if err != nil {
				t.Fatalf("reading the log through the locked handle: %v", err)
			}
			if !strings.Contains(string(b), `"id":"a"`) {
				t.Errorf("the locked handle read %q, want the line already appended", b)
			}
			return steerLines("b", "c"), nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if ids, _ := steerIDs(t, st, session); strings.Join(ids, ",") != "a,b,c" {
			t.Errorf("the log holds %v, want [a b c]", ids)
		}
	})

	t.Run("a failed build writes nothing", func(t *testing.T) {
		st, err := Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		refused := errors.New("budget unreadable")
		err = st.AppendSteer(session, func(io.ReaderAt) ([]SteerLine, error) { return steerLines("a"), refused })
		if !errors.Is(err, refused) {
			t.Fatalf("AppendSteer returned %v, want the build's error", err)
		}
		if ids, _ := steerIDs(t, st, session); len(ids) != 0 {
			t.Errorf("a failed build left %v in the log", ids)
		}
	})

	t.Run("one line over the bound writes none", func(t *testing.T) {
		st, err := Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		huge := &testSteerLine{Type: "steer", ID: "huge", Slots: json.RawMessage(`"` + strings.Repeat("x", MaxRecordBytes) + `"`)}
		lines := append(steerLines("a"), huge)
		if err := st.AppendSteer(session, fixed(lines)); !errors.Is(err, ErrRecordOversize) {
			t.Fatalf("AppendSteer with a line over MaxRecordBytes returned %v, want ErrRecordOversize", err)
		}
		if ids, _ := steerIDs(t, st, session); len(ids) != 0 {
			t.Errorf("an append refused for one line left %v in the log", ids)
		}
	})

	t.Run("no lines, no write and no sync owed", func(t *testing.T) {
		st, err := Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		st.barrier = orderingBarrier
		st.DeferDurability()
		if err := st.AppendSteer(session, fixed(nil)); err != nil {
			t.Fatal(err)
		}
		if st.dirty != "" {
			t.Errorf("an append of no lines left %q owed a sync", st.dirty)
		}
		info, err := os.Stat(filepath.Join(st.RunDir(session), FileSteer))
		if err == nil && info.Size() != 0 {
			t.Errorf("an append of no lines wrote %d bytes", info.Size())
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
	})
}

// TestREC42d_SteerLogHealsATornTail: a steer line killed mid-write leaves a
// fragment with no newline; the next append starts a line of its own, and
// the reader counts the fragment as one line it could not read.
func TestREC42d_SteerLogHealsATornTail(t *testing.T) {
	const session = "sess-steer-torn"
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AppendSteer(session, fixed(steerLines("a"))); err != nil {
		t.Fatal(err)
	}
	appendRaw(t, filepath.Join(st.RunDir(session), FileSteer), []byte(`{"type":"steer","id":"torn"`))
	if err := st.AppendSteer(session, fixed(steerLines("b", "c"))); err != nil {
		t.Fatal(err)
	}
	ids, skipped := steerIDs(t, st, session)
	if strings.Join(ids, ",") != "a,b,c" || skipped != 1 {
		t.Errorf("read %v and %d unreadable lines, want [a b c] and 1", ids, skipped)
	}
}

// TestREC42e_SteerLogIsAPrivateSideFile: steer.ndjson sits in the run's own
// directory, 0600, and a run with no steer log reads as one with no lines.
func TestREC42e_SteerLogIsAPrivateSideFile(t *testing.T) {
	const session = "sess-steer-mode"
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if ids, skipped := steerIDs(t, st, session); len(ids) != 0 || skipped != 0 {
		t.Fatalf("a run with no steer log read %v and %d unreadable", ids, skipped)
	}
	if err := st.AppendSteer(session, fixed(steerLines("a"))); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(st.Root(), DirRuns, session, "steer.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != fileMode {
		t.Errorf("steer.ndjson is %v, want %v", info.Mode().Perm(), fileMode)
	}
}

// TestREC42g_ATextOverItsCapIsCappedBeforeItsLineIsDecided: build caps a
// line's texts before it decides the line's state (Technical spec 7.2.5 G2:
// the size check runs before the append, so each line's state matches what
// printed), and such a line lands with its text withheld the store's way. A
// line that reaches the writer still over its cap refuses the whole append
// with ErrSteerTextWithheld and writes nothing: the caller prints nothing,
// so the log must hold nothing of it either (STR-10).
func TestREC42g_ATextOverItsCapIsCappedBeforeItsLineIsDecided(t *testing.T) {
	const session = "sess-steer-withheld"
	long := func() *testSteerLine {
		return &testSteerLine{Type: "steer", ID: "long", ModelText: strings.Repeat("x", MaxSteerTextBytes+1)}
	}

	t.Run("a line build did not cap writes none of the batch", func(t *testing.T) {
		st, err := Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		st.barrier = orderingBarrier
		st.DeferDurability()
		err = st.AppendSteer(session, fixed(append(steerLines("a"), long())))
		if !errors.Is(err, ErrSteerTextWithheld) {
			t.Fatalf("AppendSteer with an uncapped text over its cap returned %v, want ErrSteerTextWithheld", err)
		}
		if ids, skipped := steerIDs(t, st, session); len(ids) != 0 || skipped != 0 {
			t.Errorf("an append refused for an uncapped text left %v (%d unreadable) in the log", ids, skipped)
		}
		if st.dirty != "" {
			t.Errorf("an append that wrote nothing left %q owed a sync", st.dirty)
		}
	})

	t.Run("a line build capped lands withheld", func(t *testing.T) {
		st, err := Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		err = st.AppendSteer(session, func(io.ReaderAt) ([]SteerLine, error) {
			l := long()
			if !l.CapText() {
				t.Fatal("CapText did not withhold a text one byte over its cap")
			}
			return append(steerLines("a"), l), nil
		})
		if err != nil {
			t.Fatalf("AppendSteer of a line build capped returned %v, want nil", err)
		}
		lines, _, err := st.ReadSteer(session)
		if err != nil {
			t.Fatal(err)
		}
		if len(lines) != 2 {
			t.Fatalf("read %d lines, want both", len(lines))
		}
		var got testSteerLine
		if err := json.Unmarshal(lines[1], &got); err != nil {
			t.Fatal(err)
		}
		if got.ModelText != "" || got.ModelTextWithheld == nil || *got.ModelTextWithheld != "oversize" {
			t.Errorf("the long line reads model_text %d bytes, withheld %v; want empty and oversize", len(got.ModelText), got.ModelTextWithheld)
		}
	})
}
