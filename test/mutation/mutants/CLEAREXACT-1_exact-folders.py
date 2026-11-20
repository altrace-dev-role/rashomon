# CLEAREXACT-1: the store's exact folders (card CLEAR-EXACT1). MkdirExact
# (internal/store/exactdir.go) makes a folder under the store root one level
# at a time and refuses, with ErrNotExactDir, a level that is a link or not a
# folder, and, in the levels purge judges by exact name, a folder its parent
# lists under another case of its name. MeetClear
# (internal/store/clearfolder.go) checks its cwd, its own id and its side
# before it makes any folder, then makes clear/<digest of the cwd> through
# MkdirExact with both levels exact. Each break below undoes one of those
# promises, and the test named beside it fails under it.
X = "internal/store/exactdir.go"
C = "internal/store/clearfolder.go"

# The type check is kept and made never true, so fi stays read and the
# package still compiles.
m("CLEAREXACT-1 a link or a file at a level is taken as a folder", X,
  "\t\tif fi.Mode().Type() != fs.ModeDir {\n",
  "\t\tif fi.Mode().Type() != fs.ModeDir && false {\n",
  "TestMkdirExact_RefusesALinkOrAFileAtAnyLevel")

m("CLEAREXACT-1 another case of a name at an exact level is taken", X,
  "\t\tif i < exact {\n", "\t\tif false {\n",
  "TestMkdirExact_RefusesAnotherCaseAtAnExactLevel",
  # Only a disk that folds case finds TOP when asked for top. Linux's disks
  # do not: there the planted folder is another folder, MkdirExact makes the
  # asked one beside it with or without this break, and the test takes that
  # branch and passes. Without platform=, a Linux sweep would read this
  # mutant as not detected rather than as the darwin-only mutant it is.
  platform="darwin")

m("CLEAREXACT-1 MeetClear makes its folder before it checks its input", C,
  "\tif !filepath.IsAbs(cwd) || !isPlainSegment(self) || (side != ClearEnd && side != ClearStart) {\n"
  "\t\treturn ClearLink{}, false, nil\n\t}\n"
  "\tsum := sha256.Sum256([]byte(cwd))\n"
  "\tdir, err := MkdirExact(root, 2, DirClear, hex.EncodeToString(sum[:]))\n"
  "\tif err != nil {\n\t\treturn ClearLink{}, false, err\n\t}\n",
  "\tsum := sha256.Sum256([]byte(cwd))\n"
  "\tdir, err := MkdirExact(root, 2, DirClear, hex.EncodeToString(sum[:]))\n"
  "\tif err != nil {\n\t\treturn ClearLink{}, false, err\n\t}\n"
  "\tif !filepath.IsAbs(cwd) || !isPlainSegment(self) || (side != ClearEnd && side != ClearStart) {\n"
  "\t\treturn ClearLink{}, false, nil\n\t}\n",
  "TestMeetClear_FolderIsTheDigestOfTheCwd")
