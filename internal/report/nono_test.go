package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/nono"
)

// trailWith is an observed trail holding the given events, at one instant.
func trailWith(events ...nono.Event) nono.Observation {
	at := time.UnixMilli(1790725093166)
	for i := range events {
		events[i].At = at
	}
	return nono.Observation{Observed: true, Events: events, Sessions: 1}
}

// renderNono is the sandbox section as the text report prints it.
func renderNono(n Nono) string {
	var b bytes.Buffer
	writeNono(&b, n)
	return b.String()
}

// TestNono_AnUnknownModeIsCountedWithoutAProxyStore: the default path is no
// proxy store, and the mode counter sat below the guard that returns when
// there is none, so a transport nono added after this reader was written
// went uncounted for most users. The decision counter had already been moved
// above that guard for the same reason.
func TestNono_AnUnknownModeIsCountedWithoutAProxyStore(t *testing.T) {
	obs := trailWith(
		nono.Event{Host: "pypi.org", Port: 443, Decision: nono.DecisionAllow, Mode: "connect"},
		nono.Event{Host: "tunnel.example", Port: 443, Decision: nono.DecisionAllow, Mode: "tunnel2"},
	)
	n := buildNono(obs, Destinations{}, true, nil)
	if n.UnknownModes != 1 {
		t.Errorf("no proxy store: unknown_modes = %d, want 1", n.UnknownModes)
	}
	if out := renderNono(n); !strings.Contains(out, "    1 event carried a transport this reader does not know\n") {
		t.Errorf("the text does not report the unknown transport:\n%s", out)
	}
}

// TestNono_AnUnknownModeIsCountedOnceWithAProxyStore: with a proxy store the
// reconciliation loop runs as well, and must not count the same event again.
func TestNono_AnUnknownModeIsCountedOnceWithAProxyStore(t *testing.T) {
	obs := trailWith(
		nono.Event{Host: "tunnel.example", Port: 443, Decision: nono.DecisionAllow, Mode: "tunnel2"},
	)
	dests := Destinations{Observed: true, WindowApplied: true}
	n := buildNono(obs, dests, true, nil)
	if n.UnknownModes != 1 {
		t.Errorf("with a proxy store: unknown_modes = %d, want 1", n.UnknownModes)
	}
	// Still read as observable by the proxy, so the gap it leaves is not
	// excused as plain HTTP.
	if len(n.SawWhatTheProxyDidNot) != 1 || len(n.PlainHTTP) != 0 {
		t.Errorf("saw_what_the_proxy_did_not = %v, plain_http = %v; want the host listed and not excused",
			n.SawWhatTheProxyDidNot, n.PlainHTTP)
	}
}
