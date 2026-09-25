package posture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestParseListen is the whole grammar a listen address may have before
// anything exports it: HOST:PORT, HOST exactly one of three spellings, PORT
// ASCII digits with no sign and no leading zero, 1-65535.
//
// The address is not an opaque string once it is accepted. env prints it into
// a line the operator hands to eval, so a parse that read only the host -- the
// shape isLoopback has, and the shape that shipped -- let "127.0.0.1:1;cmd"
// through with a command attached. Every refused row below is a way text
// other than a host and a port could have reached a shell.
func TestParseListen(t *testing.T) {
	for addr, want := range map[string]Listen{
		"127.0.0.1:18080": {Host: "127.0.0.1", Port: 18080},
		"127.0.0.1:19090": {Host: "127.0.0.1", Port: 19090},
		"[::1]:18080":     {Host: "[::1]", Port: 18080},
		"localhost:8443":  {Host: "localhost", Port: 8443},
		"127.0.0.1:1":     {Host: "127.0.0.1", Port: 1},
		"127.0.0.1:65535": {Host: "127.0.0.1", Port: 65535},
	} {
		got, ok := parseListen(addr)
		if !ok || got != want {
			t.Errorf("parseListen(%q) = %+v, %v; want %+v, true", addr, got, ok, want)
		}
		// The rebuilt form is what callers export, and for every accepted
		// address it is the address itself: nothing is normalised away that a
		// reader of the status file would have expected to see.
		if ok && got.String() != addr {
			t.Errorf("parseListen(%q).String() = %q, want the address back", addr, got.String())
		}
	}

	for _, bad := range []string{
		// Portless. The proxy URL would carry no port and the client would
		// pick one, which nothing verified is listening on.
		"localhost", "::1", "[::1]", "127.0.0.1", "127.0.0.1:", "[::1]:",
		// Junk after the port: the injection rows.
		"127.0.0.1:1;cmd", "127.0.0.1:1 $(x)", "127.0.0.1:18080\ntouch /tmp/x",
		"127.0.0.1:18080`id`", "127.0.0.1:18080&", "127.0.0.1:18080|sh",
		// A port that is a number only to a lenient reader.
		"127.0.0.1:+18080", "127.0.0.1:-1", "127.0.0.1:018080", "127.0.0.1:0",
		"127.0.0.1:65536", "127.0.0.1:99999999999999999999", "127.0.0.1: 18080",
		"127.0.0.1:18080 ", "127.0.0.1:1e4", "127.0.0.1:0x50", "127.0.0.1:http",
		"127.0.0.1:70000", "127.0.0.1:١٨٠٨٠",
		// A host outside the three spellings, however loopback it may be.
		"LOCALHOST:18080", "::1:18080", "[::1:18080", "127.0.0.2:18080",
		"[0:0:0:0:0:0:0:1]:18080", " 127.0.0.1:18080", "10.0.0.5:8080",
		"evil.example:80@127.0.0.1:18080", "rashomon:tag@127.0.0.1:18080", "",
	} {
		if got, ok := parseListen(bad); ok {
			t.Errorf("parseListen(%q) = %+v, true; want it refused", bad, got)
		}
	}
}

// TestRead_RefusesAListenAddressThatIsNotHostPort drives the decision, not the
// parser: every address the grammar refuses is a refused posture, whose reason
// names the address QUOTED -- so a newline or a control character in the file
// cannot reshape the line the operator reads -- and whose Listen is empty, so
// no caller can export a refused address by reading the wrong field.
func TestRead_RefusesAListenAddressThatIsNotHostPort(t *testing.T) {
	for _, addr := range []string{
		"localhost", "::1", "127.0.0.1:1;cmd", "127.0.0.1:1 $(x)", "127.0.0.1:+18080",
		"127.0.0.1:18080\ntouch /tmp/x",
	} {
		t.Run(addr, func(t *testing.T) {
			v := Read(writeStatus(t, addr))
			if v.Export {
				t.Fatalf("a status file naming listen address %q was accepted", addr)
			}
			if want := strconv.Quote(addr); !strings.Contains(v.Reason, want) {
				t.Errorf("reason = %q, want it to name the address as %s", v.Reason, want)
			}
			if !strings.Contains(v.Reason, "not HOST:PORT") {
				t.Errorf("reason = %q, want the grammar refusal, not some other one", v.Reason)
			}
			if v.Listen != (Listen{}) {
				t.Errorf("a refused verdict carries Listen %+v", v.Listen)
			}
		})
	}
}

// TestRead_AcceptedVerdictCarriesTheParsedListener: the accepted verdict hands
// callers the parsed form, and its reason names the rebuilt address.
func TestRead_AcceptedVerdictCarriesTheParsedListener(t *testing.T) {
	v := Read(writeStatus(t, "[::1]:19090"))
	if !v.Export {
		t.Fatalf("a loopback proxy was refused: %q", v.Reason)
	}
	if want := (Listen{Host: "[::1]", Port: 19090}); v.Listen != want {
		t.Errorf("Listen = %+v, want %+v", v.Listen, want)
	}
	if !strings.Contains(v.Reason, "[::1]:19090") {
		t.Errorf("reason = %q, want it to name the verified address", v.Reason)
	}
}

// writeStatus writes a live observe-mode status file naming addr.
func writeStatus(t *testing.T, addr string) string {
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
