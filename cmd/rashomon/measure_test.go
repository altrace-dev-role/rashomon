package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/harness"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/test/perf/payload"
)

// labRoot is a temp store root holding the lab marker, so the operator's
// switches count in it.
func labRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := store.WriteLab(root); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestMS1q_SteerEnvReadsTheArm: on a lab store, the post step's port carries
// the arm RASHOMON_STEER_ARM names, "" for any value that is not off or
// steer.
func TestMS1q_SteerEnvReadsTheArm(t *testing.T) {
	root := labRoot(t)
	for v, want := range map[string]steer.Arm{"off": steer.ArmOff, "steer": steer.ArmSteer, "": "", "live": "", "OFF": ""} {
		t.Setenv(hook.EnvSteerArm, v)
		if got := steerEnv(harness.Identity{}, time.Unix(0, 0), root).Arm; got != want {
			t.Errorf("%s=%q gives arm %q, want %q", hook.EnvSteerArm, v, got, want)
		}
	}
}

// recapUnder runs one Stop's recap under RASHOMON_STEER_ARM=arm, on a store
// that holds the lab marker when lab is set, and returns what it printed,
// the record types the run holds in order, and whether recap.json was
// written.
func recapUnder(t *testing.T, arm string, lab bool) (out []byte, kinds []string, claimed bool) {
	t.Helper()
	root, cfg := t.TempDir(), t.TempDir()
	benchSetenv(t, root, cfg)
	t.Setenv(hook.EnvSteerArm, arm)
	installID := benchInstall(t, root, cfg)
	if lab {
		if err := store.WriteLab(root); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := payload.WriteSession(root, 10, payload.Prompts); err != nil {
		t.Fatal(err)
	}
	args, input := benchRecapInput(t, installID, false)
	var stdout, stderr bytes.Buffer
	if code := cmdRecap(args, bytes.NewReader(input), &stdout, &stderr); code != exitOK || stderr.Len() != 0 {
		t.Fatalf("recap under arm %q: exit %d, stderr %q", arm, code, stderr.String())
	}
	st, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(st.RunDir(benchSessionID), store.FileRecords))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var head struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(l), &head); err != nil {
			t.Fatal(err)
		}
		kinds = append(kinds, head.Type)
	}
	_, err = os.Stat(filepath.Join(root, "recap.json"))
	return stdout.Bytes(), kinds, err == nil
}

// TestMS1n_ArmOffSilencesRecap: on a lab store under arm off the Stop's
// recap prints nothing, and the records and recap.json it leaves are the
// control's: the line was decided and claimed as in every arm.
func TestMS1n_ArmOffSilencesRecap(t *testing.T) {
	control, controlKinds, controlClaimed := recapUnder(t, "", true)
	if !bytes.Contains(control, []byte(`"systemMessage"`)) || !controlClaimed {
		t.Fatalf("premise: the control printed %q and claimed %t, want the recap line and a claim", control, controlClaimed)
	}
	off, offKinds, offClaimed := recapUnder(t, "off", true)
	if len(off) != 0 {
		t.Errorf("recap under arm off printed %q, want nothing", off)
	}
	if strings.Join(offKinds, ",") != strings.Join(controlKinds, ",") || !offClaimed {
		t.Errorf("arm off left records %v and claimed %t, want the control's %v and a claim", offKinds, offClaimed, controlKinds)
	}
}

// TestMS1v_ArmNeedsALabStore: a repository's settings can set both
// switches for every session opened in it, so on a store with no lab marker
// both read as unset and recap still speaks; `measure --prepare` writes the
// marker and nothing else, after which both count.
func TestMS1v_ArmNeedsALabStore(t *testing.T) {
	plain := t.TempDir()
	t.Setenv(hook.EnvSteerArm, "off")
	t.Setenv(hook.EnvHoldout, "off")
	if arm, off := operatorEnv(plain); arm != "" || off || armOff(plain) {
		t.Errorf("no marker: operatorEnv = %q, %t and armOff %t; want both unset", arm, off, armOff(plain))
	}
	if env := steerEnv(harness.Identity{}, time.Unix(0, 0), plain); env.Arm != "" || env.HoldoutOff {
		t.Errorf("no marker: steerEnv carries arm %q, holdout off %t; want neither", env.Arm, env.HoldoutOff)
	}
	if out, _, _ := recapUnder(t, "off", false); !bytes.Contains(out, []byte(`"systemMessage"`)) {
		t.Errorf("recap under arm off on a store with no marker printed %q, want its line", out)
	}

	root := filepath.Join(t.TempDir(), "store")
	var stdout bytes.Buffer
	if err := cmdMeasure([]string{"--prepare", "--home", root}, &stdout); err != nil {
		t.Fatal(err)
	}
	names, err := os.ReadDir(root)
	if err != nil || len(names) != 1 || names[0].Name() != store.FileLab {
		t.Fatalf("--prepare left %v (%v), want %s alone", names, err, store.FileLab)
	}
	if !strings.Contains(stdout.String(), root) {
		t.Errorf("--prepare printed %q, which does not name %s", stdout.String(), root)
	}
	if arm, off := operatorEnv(root); arm != steer.ArmOff || !off {
		t.Errorf("marker: operatorEnv = %q, %t; want off, true", arm, off)
	}
	for _, bad := range [][]string{nil, {"--prepare", "--home"}, {"--prepare", "--home", ""}, {"--prepare", "x"}, {"--prepare", "--home", root, "--json"}} {
		if err := cmdMeasure(bad, &stdout); err == nil {
			t.Errorf("cmdMeasure(%q) = nil error", bad)
		}
	}
}
