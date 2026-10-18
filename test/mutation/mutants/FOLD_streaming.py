# store.Fold (internal/store/fold.go) and fold.Validate (validate.go): the
# streaming visitor B4 adds and the check that decides whether a saved
# cursor may still be trusted. Not row-pinned (as PRESIZE.py reads):
# nothing calls either in production yet, so this pins the primitives' own
# behaviour for the wiring that follows.

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

m("FOLD Validate ignores a fold's schema version",
  "internal/fold/validate.go",
  "\tif f.V != Version || f.Writer == \"\" {\n",
  "\tif false || f.Writer == \"\" {\n",
  "TestValidate_UnknownSchemaVersionInvalidates")

m("FOLD Validate accepts a fold with no writer recorded",
  "internal/fold/validate.go",
  "\tif f.V != Version || f.Writer == \"\" {\n",
  "\tif f.V != Version || false {\n",
  "TestValidate_EmptyWriterInvalidates")

m("FOLD Validate ignores a records.ndjson identity mismatch",
  "internal/fold/validate.go",
  "\tif !exists || dev != f.Dev || inode != f.Inode {\n",
  "\tif false && (exists || dev == f.Dev || inode == f.Inode) {\n",
  "TestValidate_InodeChangeInvalidates")

m("FOLD cursorValid always checks the file even at offset 0",
  "internal/fold/validate.go",
  "\tif c.Offset == 0 {\n\t\treturn true, nil\n\t}\n",
  "\tif false {\n\t\treturn true, nil\n\t}\n",
  "TestValidate_ZeroOffsetNeverFailsTheBoundaryCheck")

m("FOLD cursorValid trusts an offset past its file's size",
  "internal/fold/validate.go",
  "\tif c.Offset < 0 || c.Offset > info.Size() {\n",
  "\tif false && (c.Offset < 0 || c.Offset > info.Size()) {\n",
  "TestValidate_OffsetPastFileSizeInvalidates")

m("FOLD cursorValid never checks the line-boundary byte",
  "internal/fold/validate.go",
  "\treturn b[0] == '\\n', nil\n",
  "\treturn true, nil\n",
  "TestValidate_OffsetNotAtLineBoundaryInvalidates")
