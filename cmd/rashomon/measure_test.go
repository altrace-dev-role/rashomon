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
// the arm reads as unset and recap still speaks, while the holdout's own
// switch is untouched (a forced arm would skip its draw, so on a normal
// store it would defeat the holdout); `measure --prepare` writes the marker
// and nothing else, after which the arm counts.
func TestMS1v_ArmNeedsALabStore(t *testing.T) {
	plain := t.TempDir()
	t.Setenv(hook.EnvSteerArm, "off")
	t.Setenv(hook.EnvHoldout, "on")
	if arm := operatorArm(plain); arm != "" || armOff(plain) {
		t.Errorf("no marker: operatorArm = %q and armOff %t; want unset", arm, armOff(plain))
	}
	if env := steerEnv(harness.Identity{}, time.Unix(0, 0), plain); env.Arm != "" || !env.HoldoutOn {
		t.Errorf("no marker: steerEnv carries arm %q, holdout on %t; want no arm and the holdout still on", env.Arm, env.HoldoutOn)
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
	if arm := operatorArm(root); arm != steer.ArmOff || !armOff(root) {
		t.Errorf("marker: operatorArm = %q; want off", arm)
	}
	for _, bad := range [][]string{nil, {"--prepare", "--home"}, {"--prepare", "--home", ""}, {"--prepare", "x"}, {"--prepare", "--home", root, "--json"}} {
		if err := cmdMeasure(bad, &stdout); err == nil {
			t.Errorf("cmdMeasure(%q) = nil error", bad)
		}
	}
}

// TestMS1zh_ReadmeNamesTheMeasureVerbs: the README's command table has a row
// for each measure verb the usage screen offers, and says where
// RASHOMON_STEER_ARM counts (a lab store only), beside RASHOMON_HOLDOUT.
func TestMS1zh_ReadmeNamesTheMeasureVerbs(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	readme := string(b)
	for _, row := range []string{"| `rashomon measure --prepare [--home DIR]` |", "| `rashomon measure pair --session ID --arm off\\|steer"} {
		if !strings.Contains(readme, row) {
			t.Errorf("README has no table row starting %q", row)
		}
	}
	i := strings.Index(readme, "`RASHOMON_STEER_ARM=off`")
	if i < 0 || !strings.Contains(readme[i:min(len(readme), i+400)], "only on a lab store") || i < strings.Index(readme, "`RASHOMON_HOLDOUT=on`") {
		t.Error("README does not say, after the RASHOMON_HOLDOUT sentence, that RASHOMON_STEER_ARM counts on a lab store only")
	}
}
