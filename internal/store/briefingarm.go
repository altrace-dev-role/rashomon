package store

import (
	"os"
	"path/filepath"
)

// The continuity briefing's resume marker (card COMPACT-SLOT1, D-c). A
// resumed session's start has no channel that reaches the person (startup,
// resume and clear failed the user half), so when the transcript's last
// compaction has a briefing not yet given, the start arms the session and
// its next prompt, whose channel does, gives the briefing and takes the arm.
// The marker holds no bytes, never the person's words or the boundary they
// were said before: the prompt reads the transcript again. It is a file
// rather than a record for the reason the stale trigger's hint is: one call
// answers it, and a prompt with no marker pays one failed remove.

// ArmBriefing arms the session's next prompt to give the continuity briefing.
func (s *Store) ArmBriefing(sessionID string) error {
	if err := s.opened(); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(s.RunDir(sessionID), FileBriefingArmed), os.O_CREATE|os.O_WRONLY, fileMode)
	if err != nil {
		return err
	}
	return f.Close()
}

// TakeBriefingArm disarms the session's briefing marker and reports whether
// this call is the one that did, so of two prompts that race for one arm
// only one gives the briefing.
func (s *Store) TakeBriefingArm(sessionID string) bool {
	return os.Remove(filepath.Join(s.RunDir(sessionID), FileBriefingArmed)) == nil
}
