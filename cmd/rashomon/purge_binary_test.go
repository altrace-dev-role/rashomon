package main

import (
	"errors"
	"strings"
	"testing"
)

// TestTRU28_PurgeBinaryIsRefusedWhereItCannotBeRemoved: on Windows a running
// program cannot remove its own executable, and nowhere can purge remove a
// binary whose path it does not know, so --binary is refused before
// anything is printed or removed, naming the step that is left to the user.
func TestTRU28_PurgeBinaryIsRefusedWhereItCannotBeRemoved(t *testing.T) {
	if err := refuseBinary("linux", "/opt/bin/rashomon", nil); err != nil {
		t.Errorf("refuseBinary on linux with a known path = %v, want nil", err)
	}
	if err := refuseBinary("windows", `C:\bin\rashomon.exe`, nil); err == nil || !strings.Contains(err.Error(), "Windows") ||
		!strings.Contains(err.Error(), `Remove-Item "C:\bin\rashomon.exe"`) {
		t.Errorf("refuseBinary on windows = %v, want a refusal naming the platform and the Remove-Item command", err)
	}
	if err := refuseBinary("darwin", "", errors.New("no path")); err == nil || !strings.Contains(err.Error(), "no path") {
		t.Errorf("refuseBinary with no known path = %v, want a refusal carrying why", err)
	}
}
