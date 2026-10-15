# A baseline written before host.Canonical folded an IP literal to its one
# spelling holds the uncanonicalised string, and compared as stored, a host
# the baseline already knew reads as novel again. Skipping the re-key on
# load, accepting an unshaped key, or losing either half of the ownership
# merge is the regression.
m("RUBRIC lld-11 load does not re-key legacy hosts", "internal/baseline/baseline.go",
  "\tf.Hosts = canonicalizeHosts(f.Hosts)\n\treturn f, true, nil\n",
  "\treturn f, true, nil\n", "TestUpdate_LegacyIPLiteralIsNotNovelAgain|TestLoad_")
m("RUBRIC lld-11 an unshaped legacy key is kept", "internal/baseline/fold.go",
  "\t\tcanon, ok := host.Canonical(h)\n\t\tif !ok || !host.Shaped(canon) {\n\t\t\tcontinue\n\t\t}\n",
  "\t\tcanon, ok := host.Canonical(h)\n\t\tif !ok {\n\t\t\tcontinue\n\t\t}\n", "TestLoad_RefusesAnUnshapedLegacyKey")
m("RUBRIC lld-11 a colliding legacy key loses the later LastSeenAtUnixMS", "internal/baseline/fold.go",
  "\t\tnext.LastSeenAtUnixMS = max(lastSeen(prev), lastSeen(e))\n",
  "\t\tnext.LastSeenAtUnixMS = lastSeen(prev)\n", "TestLoad_MergesCollidingLegacyKeys")
m("RUBRIC lld-11 a colliding legacy key loses the earlier FirstSeenSession", "internal/baseline/fold.go",
  "\t\tif e.FirstSeenAtUnixMS > 0 && (prev.FirstSeenAtUnixMS == 0 || e.FirstSeenAtUnixMS < prev.FirstSeenAtUnixMS) {\n",
  "\t\tif false {\n", "TestLoad_MergesCollidingLegacyKeys")
