package acceptance

import (
	"bytes"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TRU-44 (Technical spec 7.2.9): detach checks its own work. When the
// settings file is unchanged since watch, the result must be the file as it
// was before watch, and detach says it restored it byte for byte; when it is
// not, the entries are still removed, and detach prints the bug line and a
// diff that carries no value from the file. When the file was edited since
// watch, detach says what it removed and that every other byte is as the user
// and other tools left it.

// The lines detach prints, as the spec words them.
const (
	detachBugLine = "rashomon: detach could not restore settings.json byte for byte; this is a bug, please report it"
	detachKeptFmt = "rashomon: removed rashomon's %d entries; every other byte is as you and other tools left it"
	restoredWords = "byte for byte, as it was before watch"
)

// installedEntryCount is how many entries one watch installs, read from the
// file rather than restated, so a new event does not silently change what
// the line under test is expected to say.
func installedEntryCount(t *testing.T, e *env) int {
	t.Helper()
	n := 0
	for _, groups := range e.settings().Hooks {
		n += len(ours(groups))
	}
	if n == 0 {
		t.Fatal("premise: watch installed no entries")
	}
	return n
}

// TestTRU44_DetachSelfCheckFiresOnALossyEdit: a settings file whose own
// layout the editor cannot give back -- an event the user left as an empty
// array, which detach removes along with the key watch's entry emptied -- is
// unchanged between watch and detach, so the result has to equal the
// pre-image, and it does not. (Layouts the editor splices around, such as a
// compact file with no final newline, are restored exactly: A8.) Detach still removes the entries and exits 0,
// prints the bug line, and its diff names no value the file holds.
func TestTRU44_DetachSelfCheckFiresOnALossyEdit(t *testing.T) {
	e := newEnv(t)
	const secret = "Bash(deploy-with-token-s3cr3t:*)"
	e.writeSettings(`{"permissions":{"allow":["` + secret + `"]},"hooks":{"PreToolUse":[]},"model":"opus"}`)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	res := e.detach()
	if res.exitCode != 0 {
		t.Fatalf("detach: exit %d, stderr %q; a failed self-check still exits 0", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stdout, detachBugLine+"\n") {
		t.Errorf("detach did not print the bug line:\n%s", res.stdout)
	}
	if strings.Contains(res.stdout, restoredWords) {
		t.Errorf("detach claimed a byte-for-byte restore it did not make:\n%s", res.stdout)
	}
	for _, value := range []string{"deploy-with-token", "s3cr3t", "opus"} {
		if strings.Contains(res.stdout, value) || strings.Contains(res.stderr, value) {
			t.Errorf("detach's diff carries the value %q from the file:\n%s", value, res.stdout)
		}
	}
	if !strings.Contains(res.stdout, "\n- ") || !strings.Contains(res.stdout, "\n+ ") {
		t.Errorf("detach printed no diff after the bug line:\n%s", res.stdout)
	}
	// The result is written anyway: our entries are gone, the user's values
	// are all still there.
	sf := e.settings()
	for event, groups := range sf.Hooks {
		if n := len(ours(groups)); n != 0 {
			t.Errorf("%d entries of ours remain under %s", n, event)
		}
	}
	var doc struct {
		Permissions struct {
			Allow []string `json:"allow"`
		} `json:"permissions"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal(e.settingsBytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Permissions.Allow) != 1 || doc.Permissions.Allow[0] != secret || doc.Model != "opus" {
		t.Errorf("detach lost a value: %+v", doc)
	}
}

// TestTRU44_DetachSaysItRestoredTheFileByteForByte: an absent file comes back
// absent, and a file in the layout Claude Code writes comes back as the same
// bytes, and detach says so -- never the bug line.
func TestTRU44_DetachSaysItRestoredTheFileByteForByte(t *testing.T) {
	cases := []struct {
		name string
		seed *string
	}{
		{"absent", nil},
		{"present", ptr("{\n  \"permissions\": {\n    \"allow\": [\n      \"Bash(ls:*)\"\n    ]\n  },\n  \"hooks\": {\n    \"PreToolUse\": [\n      " + foreignEntry + "\n    ]\n  }\n}\n")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			if c.seed != nil {
				e.writeSettings(*c.seed)
			}
			if res := e.watch(); res.exitCode != 0 {
				t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
			}
			res := e.detach()
			if res.exitCode != 0 {
				t.Fatalf("detach: exit %d, stderr %q", res.exitCode, res.stderr)
			}
			if !strings.Contains(res.stdout, restoredWords) || strings.Contains(res.stdout, detachBugLine) {
				t.Errorf("detach did not say it restored the file byte for byte:\n%s", res.stdout)
			}
			if c.seed == nil {
				if _, err := os.Lstat(e.settingsPath()); !os.IsNotExist(err) {
					t.Errorf("watch created settings.json and detach left it (stat: %v):\n%s", err, e.settingsBytes())
				}
				return
			}
			if got := e.settingsBytes(); !bytes.Equal(got, []byte(*c.seed)) {
				t.Errorf("settings.json is not the pre-image:\n got %q\nwant %q", got, *c.seed)
			}
		})
	}
}

// TestTRU44_DetachAfterAForeignEditNamesWhatItRemoved: Claude Code rewrote
// the file after watch, so it is not the file watch left and there is no
// pre-image to compare with. Detach removes the entries, keeps the edit, and
// says how many entries it removed -- never the bug line, never a claim of a
// byte-for-byte restore.
func TestTRU44_DetachAfterAForeignEditNamesWhatItRemoved(t *testing.T) {
	for _, how := range []string{"plain", "--install"} {
		t.Run(how, func(t *testing.T) {
			e := newEnv(t)
			e.writeSettings("{\n  \"model\": \"opus\"\n}\n")
			if res := e.watch(); res.exitCode != 0 {
				t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
			}
			n := installedEntryCount(t, e)
			var doc map[string]any
			if err := json.Unmarshal(e.settingsBytes(), &doc); err != nil {
				t.Fatal(err)
			}
			doc["permissions"] = map[string]any{"allow": []string{"Bash(git log:*)"}}
			out, err := json.MarshalIndent(doc, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			e.writeSettings(string(out) + "\n")

			args := []string{"detach"}
			if how == "--install" {
				args = append(args, "--install", e.installID())
			}
			res := e.run("", nil, args...)
			if res.exitCode != 0 {
				t.Fatalf("detach: exit %d, stderr %q", res.exitCode, res.stderr)
			}
			want := strings.Replace(detachKeptFmt, "%d", strconv.Itoa(n), 1)
			if !strings.Contains(res.stdout, want+"\n") {
				t.Errorf("detach did not say %q:\n%s", want, res.stdout)
			}
			if strings.Contains(res.stdout, detachBugLine) || strings.Contains(res.stdout, restoredWords) {
				t.Errorf("detach after a foreign edit printed the self-check's other lines:\n%s", res.stdout)
			}
			if !strings.Contains(string(e.settingsBytes()), `"Bash(git log:*)"`) {
				t.Errorf("the edit made after watch did not survive detach:\n%s", e.settingsBytes())
			}
		})
	}
}
