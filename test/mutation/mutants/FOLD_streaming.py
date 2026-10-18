# store.Fold (internal/store/fold.go): the streaming visitor B4 adds, read
# from a byte offset to a record file's last complete line. Not row-pinned
# (as PRESIZE.py reads): nothing calls Fold in production yet, so this pins
# the primitive's own behaviour for the wiring that follows.

m("FOLD a missing file is read as an error instead of folding nothing",
  "internal/store/fold.go",
  "\tif errors.Is(err, fs.ErrNotExist) {\n\t\treturn offset, 0, nil\n\t}\n",
  "\tif errors.Is(err, fs.ErrPermission) {\n\t\treturn offset, 0, nil\n\t}\n",
  "TestFoldStore_FoldOnMissingFileFoldsNothing")

m("FOLD an offset past the file's size is trusted instead of rejected",
  "internal/store/fold.go",
  "\tif offset < 0 || offset > size {\n",
  "\tif false {\n",
  "TestFoldStore_FoldRejectsAnOffsetPastTheFilesSize")

m("FOLD every Fold call rescans the whole file instead of resuming",
  "internal/store/fold.go",
  "\tif offset == size {\n\t\treturn offset, size, nil\n\t}\n",
  "\tif true {\n\t\treturn offset, size, nil\n\t}\n",
  "TestFoldStore_FoldStreamsFromOffset")

m("FOLD a torn tail is folded instead of left for the next call",
  "internal/store/fold.go",
  "\t\t\tif errors.Is(rerr, io.EOF) {\n"
  "\t\t\t\t// A torn tail: the file ended before this line did. Leave\n"
  "\t\t\t\t// it for the next Fold call rather than folding a line\n"
  "\t\t\t\t// that may still be half-written.\n"
  "\t\t\t\treturn folded, nil\n"
  "\t\t\t}\n",
  "\t\t\tif errors.Is(rerr, io.EOF) {\n"
  "\t\t\t\tfn(line)\n"
  "\t\t\t\treturn pos, nil\n"
  "\t\t\t}\n",
  "TestFoldStore_FoldLeavesATornTailForNextTime")

m("FOLD a line over maxLine is never reported as oversize",
  "internal/store/fold.go",
  "\t\t\tif len(line)+len(chunk) > maxLine {\n",
  "\t\t\tif false {\n",
  "TestFoldStore_FoldOverlongLineReadsAsNil")
