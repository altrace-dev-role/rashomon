package acceptance

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TRU-1 and TRU-28 (Technical spec 7.2.3 step 6, 7.4): watch opens the
// store and records the directories it created before it edits
// settings.json, and a watch that cannot record what its edit changed undoes
// that edit rather than leaving entries detach cannot check.

// faultWatchRecord is the point between watch's settings write and its
// record of what that write changed.
const faultWatchRecord = "watch.record"

// TestTRU1_AWatchThatCannotRecordItsEditRollsItBack: the record of the
// edit fails, so watch puts settings.json back as it found it -- the same
// bytes, or no file where there was none -- says so, and exits 1.
func TestTRU1_AWatchThatCannotRecordItsEditRollsItBack(t *testing.T) {
	for name, seed := range map[string]string{
		"settings existed": "{\n  \"model\": \"opus\"\n}\n",
		"no settings":      "",
	} {
		t.Run(name, func(t *testing.T) {
			e := newPurgeEnv(t)
			if seed != "" {
				writeFiles(t, e.settingsPath())
				e.writeSettings(seed)
			}
			res := e.watch("RASHOMON_FAULT=" + faultWatchRecord + ":fail=1")
			if res.exitCode != 1 {
				t.Fatalf("watch with a failing record: exit %d, want 1\nstdout:\n%s\nstderr: %q", res.exitCode, res.stdout, res.stderr)
			}
			if got := e.settingsBytes(); !bytes.Equal(got, []byte(seed)) {
				t.Errorf("settings.json after the rollback = %q, want %q", got, seed)
			}
			if seed == "" {
				if _, err := os.Lstat(e.settingsPath()); err == nil {
					t.Errorf("the rollback left a settings.json where there was none")
				}
			}
			if !strings.Contains(res.stderr, "put back") {
				t.Errorf("watch does not say it put settings.json back: %q", res.stderr)
			}
			if strings.Contains(res.stdout, "watching") {
				t.Errorf("a rolled-back watch says it is watching:\n%s", res.stdout)
			}
		})
	}
}

// TestTRU28_WatchRecordsCreatedDirsBeforeItEditsSettings: a watch that dies
// right after its settings write has already recorded the directories it
// created, so a later purge can remove them.
func TestTRU28_WatchRecordsCreatedDirsBeforeItEditsSettings(t *testing.T) {
	e := newPurgeEnv(t)
	res := e.watch("RASHOMON_FAULT=" + faultWatchRecord + ":plain_panic")
	if res.exitCode == 0 {
		t.Fatalf("watch with a panic after its settings write: exit 0\nstdout:\n%s", res.stdout)
	}
	if !strings.Contains(string(e.settingsBytes()), " --install ") {
		t.Fatalf("premise: the settings write did not happen before the panic: %q", e.settingsBytes())
	}
	body, err := os.ReadFile(filepath.Join(e.home, "install.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rec struct {
		CreatedDirs []string `json:"created_dirs"`
	}
	if err := json.Unmarshal(body, &rec); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{e.home, e.configDir, filepath.Join(e.fakeHome, ".local")} {
		if !slices.Contains(rec.CreatedDirs, want) {
			t.Errorf("created_dirs %q does not name %s", rec.CreatedDirs, want)
		}
	}
}
