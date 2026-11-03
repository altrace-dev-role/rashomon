package main

import (
	"bytes"
	"strings"
	"testing"
)

// An agent session cannot delete the record: purge, with or without --yes and
// --binary, refuses it, names the command to run in a terminal outside Claude
// Code, and changes nothing -- not the store root, and not settings.json,
// which purge's detach rewrites before anything is removed (#177 review T1).
func TestAGPU1_AnAgentSessionCannotPurge(t *testing.T) {
	root, cfg, _ := agentHere(t)
	before := treeState(t, root, cfg)
	t.Setenv("CLAUDECODE", "1")
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"purge"}, "purge"},
		{[]string{"purge", "--yes"}, "purge"},
		{[]string{"purge", "--yes", "--binary"}, "purge --binary"},
	} {
		var stdout, stderr bytes.Buffer
		code := run(c.args, nil, &stdout, &stderr)
		if code != exitFail || stdout.Len() != 0 {
			t.Errorf("rashomon %s in an agent session: exit %d, stdout %q, stderr %q", strings.Join(c.args, " "), code, stdout.String(), stderr.String())
		}
		wantRefusal(t, strings.Join(c.args, " "), stderr.String(), c.want, "removed")
	}
	if after := treeState(t, root, cfg); after != before {
		t.Errorf("an agent session changed the store root or settings.json:\n%s\nwas:\n%s", after, before)
	}
}
