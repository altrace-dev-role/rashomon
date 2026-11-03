# INDEX-POINTER-FILE1 (R29): v1.2.0-alpha.1 reads the project index with
# DisallowUnknownFields, so the last two checkpoints moved out of it into a
# file of their own. IPF is not a row prefix, so the file claims no row, as
# RP-2 does. Each break puts a key alpha.1 refuses back in the index, loses
# the checkpoint file's write or read, makes its reader as strict as alpha.1's,
# lifts one of its caps, or leaves it out of purge or the brief.
E = "internal/verify/evaluate.go"
I = "internal/verify/index.go"
F = "internal/verify/index_pointer.go"
S = "cmd/rashomon/recap_stop.go"
m("IPF-1 the index gains a key alpha.1 does not name", E,
  "\tEntries    []IndexEntry `json:\"entries\"`\n",
  "\tEntries    []IndexEntry `json:\"entries\"`\n\tLater      string       `json:\"later,omitempty\"`\n", "TestIPF1b_")
m("IPF-1 the index is written with a key alpha.1 refuses", I,
  "\treturn append(b, '\\n'), nil\n", "\treturn append(append(b[:len(b)-1], `,\"last\":{}}`...), '\\n'), nil\n", "TestIPF1_")
m("IPF-1 the Stop writes no checkpoint file", S, "\t_ = verify.WriteCheckpoints(path, c)\n", "", "TestIPF1_")
m("IPF-1 the Stop notes over the file without reading it", S,
  "\tc, _ := verify.ReadCheckpoints(path)\n", "\tc, _ := (*verify.Checkpoints)(nil), path\n", "TestIPF1_")
m("IPF-1 the checkpoint file's reader refuses a later key", F,
  "\tdec := json.NewDecoder(lr)\n", "\tdec := json.NewDecoder(lr)\n\tdec.DisallowUnknownFields()\n", "TestIPF1c_")
m("IPF-1 the checkpoint file's read cap is lifted", F, "\tif lr.N == 0 {\n", "\tif false {\n", "TestIPF1c_")
m("IPF-1 the checkpoint file's write cap is lifted", F,
  "\tif b = append(b, '\\n'); len(b) > MaxCheckpointsBytes {\n", "\tif b = append(b, '\\n'); false {\n", "TestIPF1c_")
m("IPF-1 purge leaves the checkpoint files", "internal/purge/purge.go",
  "(name == verify.ProjectsDir || name == verify.CheckpointsDir)", "(name == verify.ProjectsDir)", "TestTRU28b_")
m("IPF-1 the brief reads no checkpoint file", "cmd/rashomon/brief.go",
  "\tptr := c.PreviousOf(sessionID)\n", "\tptr := c.PreviousOf(sessionID)\n\tptr = nil\n", "TestRP2t_|TestRP2zb_")
