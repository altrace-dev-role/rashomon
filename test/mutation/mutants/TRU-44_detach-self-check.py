# TRU-44: detach checks its own work against the hashes watch recorded, and
# its diff carries no value from the settings file.
m("TRU-44 any result counts as the pre-image", "cmd/rashomon/detach.go",
  "!edit.removed && *rec.SettingsExisted && sha256Hex(edit.after) == rec.PreSHA256",
  "!edit.removed && *rec.SettingsExisted",
  "TestTRU44_")
m("TRU-44 an edited file counts as unchanged since watch", "cmd/rashomon/detach.go",
  "\t\texisted && sha256Hex(before) == rec.PostSHA256\n",
  "\t\texisted\n",
  "TestTRU44_")
m("TRU-44 a file watch created is kept when restored", "cmd/rashomon/detach.go",
  "\t\treturn unchangedSinceWatch(rec, before, true) && !*rec.SettingsExisted\n",
  "\t\treturn false\n",
  "TestTRU44_")
m("TRU-44 a first watch records no pre-image", "cmd/rashomon/detach.go",
  "\t\tcase !hadOurs:\n",
  "\t\tcase false:\n",
  "TestTRU44_")
m("TRU-44 watch records the file it read as the file it left", "cmd/rashomon/detach.go",
  "\t\tr.PostSHA256 = sha256Hex(edit.after)\n",
  "\t\tr.PostSHA256 = sha256Hex(edit.before)\n",
  "TestTRU44_")
m("TRU-44 a failed self-check prints no bug line", "cmd/rashomon/detach.go",
  "\t\tfmt.Fprintln(w, detachBugLine)\n\t\tgot := \"no file\"\n",
  "\t\tgot := \"no file\"\n",
  "TestTRU44_")
m("TRU-44 an added byte passes as a deletion", "internal/install/removed.go",
  "\t\tif i == len(before) {\n\t\t\treturn false\n\t\t}\n",
  "\t\tif i == len(before) {\n\t\t\treturn true\n\t\t}\n",
  "TestTRU44_")
m("TRU-44 any deletion passes as ours", "internal/install/removed.go",
  "\treturn reflect.DeepEqual(doc, a)\n",
  "\treturn reflect.DeepEqual(doc, doc)\n",
  "TestTRU44_")
m("TRU-44 the diff prints string values", "internal/settings/diff.go",
  "\t\t\t} else {\n\t\t\t\tb.WriteString(Elided)\n\t\t\t}\n",
  "\t\t\t} else {\n\t\t\t\tb.WriteString(line[i : end+1])\n\t\t\t}\n",
  "TestTRU44_")
m("TRU-44 an install record update can change the install id", "internal/store/install_record.go",
  "\tedit(&rec)\n\trec.InstallID = id\n",
  "\tedit(&rec)\n\t_ = id\n",
  "TestTRU44_")
