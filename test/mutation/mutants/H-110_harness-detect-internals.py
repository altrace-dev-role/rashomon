# internal/harness: the decision points inside Detect that back no single
# numbered row by themselves (Caps, the version charset gate, the
# transcript-containment guard, evidence de-duplication). H-110 names no
# requirement; these pin behaviour TestBRD1/TestBRD2 do not reach.

m("H-110 Caps grants mirror_in_band on the sdk surface",
  "internal/harness/caps.go",
  "\t\tif s == SurfaceSDK || s == SurfaceUnknown {\n\t\t\tout[CapMirrorInBand] = Unverified\n\t\t}",
  "\t\tif false {\n\t\t\tout[CapMirrorInBand] = Unverified\n\t\t}",
  "TestDetect_BlindTracksCaps")

m("H-110 Cursor and Codex get every capability by default",
  "internal/harness/caps.go",
  "\tdefault: // Cursor, Codex, Unknown, and any future value\n\t\tfor _, c := range allCapabilities {\n\t\t\tout[c] = No\n\t\t}",
  "\tdefault: // Cursor, Codex, Unknown, and any future value\n\t\tfor _, c := range allCapabilities {\n\t\t\tout[c] = Yes\n\t\t}",
  "TestDetect_BlindTracksCaps")

m("H-110 a too-long cursor_version is still stored",
  "internal/harness/detect.go",
  "\tif !present || len(v) == 0 || len(v) > 32 {",
  "\tif !present || len(v) == 0 {",
  "TestDetect_VersionCharsetGating")

m("H-110 a space passes the cursor_version charset",
  "internal/harness/detect.go",
  "\tcase c == '.' || c == '+' || c == '-':",
  "\tcase c == '.' || c == '+' || c == '-' || c == ' ':",
  "TestDetect_VersionCharsetGating")

m("H-110 transcript containment is a bare prefix, not a path relation",
  "internal/harness/detect.go",
  "\trel, err := filepath.Rel(projects, filepath.Clean(transcriptPath))\n\tif err != nil {\n\t\treturn false\n\t}\n\tif rel == \".\" {\n\t\treturn false // the transcript path names the directory itself, not a file in it\n\t}\n\treturn rel != \"..\" && !strings.HasPrefix(rel, \"..\"+string(filepath.Separator))",
  "\treturn strings.HasPrefix(filepath.Clean(transcriptPath), projects)",
  "TestDetect_TranscriptPathTraversalNeverCountsAsUnderProjects")

m("H-110 evidence is never de-duplicated",
  "internal/harness/detect.go",
  "\t\tif !seen[e] {\n\t\t\tseen[e] = true\n\t\t\tout = append(out, e)\n\t\t}",
  "\t\t_ = seen\n\t\tout = append(out, e)",
  "TestSortedEvidence_DedupesAndSorts")

m("H-110 evidence is never sorted",
  "internal/harness/detect.go",
  "\tsort.Slice(out, func(i, j int) bool { return out[i] < out[j] })",
  "\t_ = sort.Slice",
  "TestDetect_CursorEvidenceIsSortedWhenBothMarkersFire")
