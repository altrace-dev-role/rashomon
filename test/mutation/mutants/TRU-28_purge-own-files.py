# TRU-28: purge removes only the entries rashomon writes, also inside runs and
# baseline, which a user's own directory may share (RASHOMON_HOME).
m("TRU-28 a file in a run directory that rashomon does not write is planned for removal", "internal/purge/purge.go",
  "\tif level != 1 {\n\t\treturn false\n\t}\n",
  "\tif level != 1 || true {\n\t\treturn true\n\t}\n",
  "TestTRU28_")
m("TRU-28 a baseline entry that is not a baseline file is planned for removal", "internal/purge/purge.go",
  "\t\treturn isBaselineFile(name) || name == baseline.LockName",
  "\t\treturn true || isBaselineFile(name) || name == baseline.LockName",
  "TestTRU28_")
m("TRU-28 a symlink or directory under runs or baseline is taken as rashomon's", "internal/purge/purge.go",
  "\t\t\tcase ent.Type().IsRegular() && ownTreeFile(ent.Name(), isRuns, level):\n",
  "\t\t\tcase ownTreeFile(ent.Name(), isRuns, level):\n",
  "TestTRU28_")
m("TRU-28 a directory that holds a foreign path is still planned for removal", "internal/purge/purge.go",
  "\t\t\t\tif ok {\n\t\t\t\t\town = append(own, path)\n",
  "\t\t\t\tif true {\n\t\t\t\t\town = append(own, path)\n",
  "TestTRU28_")
m("TRU-28 a runs or baseline that is a symlink is walked", "internal/purge/purge.go",
  "\tif !info.IsDir() {\n\t\treturn nil, []string{top}, nil\n",
  "\tif false {\n\t\treturn nil, []string{top}, nil\n",
  "TestTRU28_")
m("TRU-28 a link or directory at the name of a file rashomon writes is planned for removal", "internal/purge/purge.go",
  "\t\tcase ownName(name) && ent.Type().IsRegular():\n",
  "\t\tcase ownName(name):\n",
  "TestTRU28_")
