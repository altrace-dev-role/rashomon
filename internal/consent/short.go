package consent

import "github.com/altrace-dev-role/rashomon/internal/disclose"

// MaxShortLines is the short screen's ceiling (CONSENT-SHORT1).
const MaxShortLines = 24

// ShortCommands are the short screen's closing lines.
var ShortCommands = []Command{
	{"Stop the notes to your agent", "rashomon steer off --user"},
	{"Stop recording", "rashomon pause"},
	{"Remove the hooks", "rashomon detach"},
	{"Remove everything", "rashomon purge"},
	{"Every detail, section by section", "rashomon consent --full"},
}

// Short is the five-section screen until CONSENT-SHORT1 writes the short one.
func Short(inv disclose.Inventory, in ScreenInput) ([]byte, error) { return Screen(inv, in) }

// Covers answers no until PLUGIN-CONSENT1 reads the records.
func Covers([]Record) bool { return false }
