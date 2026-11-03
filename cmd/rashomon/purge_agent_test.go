package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An agent session cannot delete the record: purge, with or without --yes and
// --binary, refuses it, names the command to run in a terminal, and removes
// nothing from the store root.
func TestAGPU1_AnAgentSessionCannotPurge(t *testing.T) {
	root := consentHere(t)
	list := func() string {
		var names []string
		_ = filepath.WalkDir(root, func(p string, _ os.DirEntry, _ error) error {
			names = append(names, p)
			return nil
		})
		return strings.Join(names, "\n")
	}
	before := list()
	t.Setenv("CLAUDECODE", "1")
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"purge"}, "`rashomon purge`"},
		{[]string{"purge", "--yes"}, "`rashomon purge`"},
		{[]string{"purge", "--yes", "--binary"}, "`rashomon purge --binary`"},
	} {
		var stdout, stderr bytes.Buffer
		code := run(c.args, nil, &stdout, &stderr)
		if code != exitFail || stdout.Len() != 0 || !strings.Contains(stderr.String(), "agent session; nothing was removed") || !strings.Contains(stderr.String(), c.want) {
			t.Errorf("rashomon %s in an agent session: exit %d, stdout %q, stderr %q", strings.Join(c.args, " "), code, stdout.String(), stderr.String())
		}
	}
	if after := list(); after != before {
		t.Errorf("an agent session changed the store root:\n%s\nwas:\n%s", after, before)
	}
}
