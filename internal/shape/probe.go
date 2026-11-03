package shape

// probes reports a shell line every one of whose commands, as Commands reads
// them, asks whether programs are installed and runs none (probe): `which
// pytest`, `which jq; type -P yq` (issue #52). A line with any other command
// is none, since its exit may be that command's: `which pytest && pytest -q`
// exits 1 when a test failed. A line the reading stops on, or one of no
// command, is none, so a line read in part never sets a failure aside.
func probes(line string) bool {
	cmds, ok := Commands(line)
	if !ok || len(cmds) == 0 {
		return false
	}
	for _, c := range cmds {
		if !probe(c.Words) {
			return false
		}
	}
	return true
}

// probe reports a command that asks whether programs are installed and runs
// none: which or type with a name, or command with -v or -V and a name.
// `command pytest` runs pytest, so it is no probe.
func probe(w []string) bool {
	switch {
	case len(w) < 2:
		return false
	case w[0] == "which" || w[0] == "type":
		return true
	default:
		return w[0] == "command" && (w[1] == "-v" || w[1] == "-V") && len(w) > 2
	}
}
