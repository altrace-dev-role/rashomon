# Rubric pause-9: a filesystem fault on the store root is not the same
# thing as a pause marker it cannot read, and the two must stay
# distinguishable through checkPaused, Resume and status alike.
#
# checkPaused used to read ANY store.Paused error as a pause -- a missing
# or inaccessible store root, a `pause` that is a directory, included --
# which turned a filesystem fault into a silent, unrecorded stop with
# nothing on stderr. Only a marker that is itself an unreadable plain file
# earns that answer now; everything else falls through so the real error
# surfaces through the normal path.
m("pause-9 checkPaused folds an unrelated fault into the marker case", "cmd/rashomon/main.go",
  "\tcase pausedErr != nil && !store.MarkerUnreadable(root):\n\t\treturn false\n",
  "",
  "TestCheckPaused_FaultOnStoreRootIsNotAPause")

# Resume used to return store.Paused's raw read error before ever reaching
# the marker file, so an unreadable marker -- the one state checkPaused now
# treats as a pause -- could never be cleared by the command that exists to
# clear a pause.
m("pause-9 Resume cannot clear an unreadable marker", "internal/store/pause.go",
  "\t\tif !MarkerUnreadable(root) {\n\t\t\treturn false, time.Time{}, err\n\t\t}\n",
  "\t\treturn false, time.Time{}, err\n",
  "TestResume_RemovesAnUnreadableMarker")

# RecordingState used to render an unreadable marker as a Go error, which
# statusRecording then printed as "unknown" -- even though checkPaused was
# already treating the very same marker as a pause. status had an answer
# and did not say it.
m("pause-9 RecordingState renders an unreadable marker as unknown", "cmd/rashomon/main.go",
  "\t\tif store.MarkerUnreadable(root) {\n\t\t\treturn RecordingPausedUnreadable, time.Time{}, nil\n\t\t}\n",
  "",
  "TestH82_RecordingStateDistinguishesPausedAbsentAndUnknown")

# Even with RecordingState correct, statusRecording's own switch has to act
# on RecordingPausedUnreadable -- dropping the case sends it through the
# default branch, which prints bare "paused" with no mention of why status
# cannot say more than that.
m("pause-9 statusRecording has no case for an unreadable marker", "cmd/rashomon/main.go",
  "\tcase state == RecordingPausedUnreadable:\n\t\tfmt.Fprintf(stdout, \"  recording: %s\\n\", RecordingPausedUnreadable)\n",
  "",
  "TestStatusRecording_UnreadableMarkerPrintsMarkerUnreadable")
