# RES-15: without a valid fold, Stop runs the full streaming fallback.
# fold.Validate (internal/fold/validate.go) is the check that decides
# "valid" for that sentence -- Archive 85.3.5 step 2, and the condition
# RES-15's own "without a valid fold" clause names. Nothing calls Validate
# in production yet, so these mutants pin the check's own behaviour, not
# RES-15's fallback or its 2 s deadline, which this PR does not build or
# claim (see TestRES15_FallbackPastDeadlineSaysUnavailable, still unwritten).

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
