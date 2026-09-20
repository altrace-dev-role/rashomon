package posture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRead_RefusesANonLoopbackProxy drives the real decision, not the helper.
func TestRead_RefusesANonLoopbackProxy(t *testing.T) {
	write := func(t *testing.T, addr string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "status.json")
		body, err := json.Marshal(map[string]any{
			"product": Product, "connect_mode": ModeObserve,
			"listen_addr": addr, "pid": os.Getpid(),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
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
