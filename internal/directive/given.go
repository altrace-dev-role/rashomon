package directive

// Given reports whether a session's steer lines hold the continuity briefing
// under key for agent. Red: never.
func Given(lines [][]byte, key, agent string) bool {
	return false
}
