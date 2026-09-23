package posture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestRead_RefusesANonLoopbackProxy drives the real decision, not the helper.
func TestRead_RefusesANonLoopbackProxy(t *testing.T) {
	write := func(t *testing.T, addr string) string {
		t.Helper()
		return writeDoc(t, map[string]any{
			"product": Product, "connect_mode": ModeObserve,
			"listen_addr": addr, "pid": os.Getpid(),
		})
	}

	v := Read(write(t, "10.0.0.5:8080"))
	if v.Export {
		t.Error("a status file naming a remote host was accepted; every request and the " +
			"session tag would leave this machine in cleartext")
	}
	if !strings.Contains(v.Reason, "non-loopback") {
		t.Errorf("reason = %q, want it to name the refusal", v.Reason)
	}

	if v := Read(write(t, "127.0.0.1:18080")); !v.Export {
		t.Errorf("a loopback proxy was refused: %q", v.Reason)
	}
}

// forgedLine is a second line a status file might try to print under this
// program's name, through a newline in a field a reason repeats.
const forgedLine = "\nrashomon: observe-mode proxy at 127.0.0.1:18080 is running"

// TestRead_AReasonNeverRepeatsAFieldRaw: the two reasons that repeat a field of
// the file -- the address a non-loopback refusal names and the mode an
// enforcing refusal names -- repeat it QUOTED. A raw newline there would give
// the file a line of its own on the operator's terminal, reading as this
// program saying the proxy is running.
func TestRead_AReasonNeverRepeatsAFieldRaw(t *testing.T) {
	for _, tc := range []struct {
		name, field, value, want string
	}{
		{name: "non-loopback address", field: "listen_addr", value: "10.0.0.5:8080" + forgedLine,
			want: "non-loopback"},
		{name: "mode", field: "connect_mode", value: "enforce" + forgedLine, want: "not observe"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := map[string]any{
				"product": Product, "connect_mode": ModeObserve,
				"listen_addr": "127.0.0.1:18080", "pid": os.Getpid(),
			}
			doc[tc.field] = tc.value
			v := Read(writeDoc(t, doc))
			if v.Export {
				t.Fatalf("a status file with %s %q was accepted", tc.field, tc.value)
			}
			if !strings.Contains(v.Reason, tc.want) {
				t.Errorf("reason = %q, want the %s refusal", v.Reason, tc.name)
			}
			if !strings.Contains(v.Reason, strconv.Quote(tc.value)) {
				t.Errorf("reason = %q, want it to repeat %s as %s", v.Reason, tc.field,
					strconv.Quote(tc.value))
			}
			if strings.ContainsAny(v.Reason, "\n\r") {
				t.Errorf("reason = %q carries a raw line break from the file", v.Reason)
			}
		})
	}
}

// writeDoc writes a status document and returns its path.
func writeDoc(t *testing.T, fields map[string]any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "status.json")
	body, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
