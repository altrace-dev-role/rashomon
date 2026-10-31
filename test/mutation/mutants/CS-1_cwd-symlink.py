# CS-1: a run's location is its directory as it is on disk (card
# CWD-SYMLINK1). A declared gate is located at the project key, which resolves
# symlinks; a run is located at the cwd the payload spelled, so under a symlink
# the two never met. hook.Locate digests the start directory resolved, once.
# Each break removes one rule and names the tests that hold it.
H = "internal/hook/handle.go"
m("CS-1 the start directory is digested as spelled", H,
  "shape.CWDDigest(key, physicalDir(startDirectory(cwd, dirs)))",
  "shape.CWDDigest(key, startDirectory(cwd, dirs))",
  "TestCS1_|TestH154_")
m("CS-1 physicalDir never resolves", H,
  "	if resolved, err := evalSymlinks(dir); err == nil {\n		return resolved\n	}\n	return dir\n",
  "	return dir\n",
  "TestCS1_|TestCS1c_|TestH154_")
m("CS-1 a directory that does not resolve digests as unknown", H,
  "		return resolved\n	}\n	return dir\n}",
  "		return resolved\n	}\n	return \"\"\n}",
  "TestCS1b_")
m("CS-1 only the payload cwd is resolved, before the cd steps fold onto it", H,
  "shape.CWDDigest(key, physicalDir(startDirectory(cwd, dirs)))",
  "shape.CWDDigest(key, startDirectory(physicalDir(cwd), dirs))",
  "TestCS1_LocateKeys")
m("CS-1 a line with no cd step resolves the directory again", H,
  "	if len(dirs) == 0 {\n		return s, shape.CWDDigest(key, physical)\n	}\n",
  "",
  "TestCS1d_")
m("CS-1 the no-cd location is the cwd as spelled", H,
  "return s, shape.CWDDigest(key, physical)\n",
  "return s, shape.CWDDigest(key, cwd)\n",
  "TestCS1_|TestH154_")
