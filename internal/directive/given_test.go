package directive

import "testing"

// TestCSL1_GivenIsOneBriefingPerSessionAgentAndCompaction: a briefing is
// given once per session, agent and compaction (D-b): a continuity-briefing
// line under the boundary's key, for the same agent (none for the main
// agent), delivered or withheld by arm off, whose shadow decision gave it. A
// line suppressed on its way is not given, nor one under another key, of
// another pattern or for another agent, nor a line that is not JSON. Break:
// drop any of the four tests and this fails.
func TestCSL1_GivenIsOneBriefingPerSessionAgentAndCompaction(t *testing.T) {
	const (
		delivered  = `{"pattern":"continuity-briefing","key":"b-2","state":"delivered","suppressed_by":null,"agent_id":null}`
		armOff     = `{"pattern":"continuity-briefing","key":"b-2","state":"suppressed","suppressed_by":"arm_off","agent_id":null}`
		unverified = `{"pattern":"continuity-briefing","key":"b-2","state":"suppressed","suppressed_by":"unverified_channel","agent_id":null}`
		earlier    = `{"pattern":"continuity-briefing","key":"b-1","state":"delivered","suppressed_by":null,"agent_id":null}`
		other      = `{"pattern":"task-preflight","key":"b-2","state":"delivered","suppressed_by":null,"agent_id":null}`
		agent      = `{"pattern":"continuity-briefing","key":"b-2","state":"delivered","suppressed_by":null,"agent_id":"a-1"}`
		broken     = `{"pattern":"continuity-briefing","key":"b-2","state":"delivered"`
	)
	for _, c := range []struct {
		name       string
		lines      []string
		key, agent string
		want       bool
	}{
		{"delivered", []string{earlier, delivered}, "b-2", "", true},
		{"withheld by arm off", []string{armOff}, "b-2", "", true},
		{"suppressed on its way", []string{unverified}, "b-2", "", false},
		{"an earlier compaction's", []string{earlier}, "b-2", "", false},
		{"another pattern's", []string{other}, "b-2", "", false},
		{"a subagent's, asked for the main agent", []string{agent}, "b-2", "", false},
		{"a subagent's, asked for it", []string{delivered, agent}, "b-2", "a-1", true},
		{"the main agent's, asked for a subagent", []string{delivered}, "b-2", "a-1", false},
		{"not JSON", []string{broken}, "b-2", "", false},
		{"no lines", nil, "b-2", "", false},
	} {
		var lines [][]byte
		for _, l := range c.lines {
			lines = append(lines, []byte(l))
		}
		if got := Given(lines, c.key, c.agent); got != c.want {
			t.Errorf("%s: Given = %v, want %v", c.name, got, c.want)
		}
	}
}
