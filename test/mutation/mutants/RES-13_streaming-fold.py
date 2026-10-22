# RES-13: Stop flat in session size. store.Fold (internal/store/fold.go) is
# the streaming visitor that gives a fold its bounded-memory read -- the
# mechanism RES-13's own TestRES13_StopIsFlatInSessionSize and
# TestRES13b_OnePromptSessionIsFlat will measure once a later PR wires it
# into Stop -- and fold.Write is the bound on its persisted form: a
# fold.json that could grow without limit would undermine the same
# flatness for whichever Stop reads it next. Nothing calls either in
# production yet, so these mutants pin the two primitives' own behaviour,
# not RES-13's production memory ceiling, which this PR does not claim.

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

m("FOLD an empty line is reported as fn(nil), the same signal as oversize",
  "internal/store/fold.go",
  "\t\tcase len(line) > 0:\n\t\t\tfn(line)\n",
  "\t\tcase true:\n\t\t\tfn(line)\n",
  "TestFoldStore_FoldDropsEmptyLines")

m("FOLD never lets size catch up to an offset a racing write pushed past it",
  "internal/store/fold.go",
  "\tif folded > size {\n\t\tsize = folded\n\t}\n",
  "\tif false {\n\t\tsize = folded\n\t}\n",
  "TestFoldStore_SizeNeverTrailsTheOffsetItReturns")

m("FOLD Write never refuses a document over MaxBytes",
  "internal/fold/write.go",
  "\tif len(b) > MaxBytes {\n",
  "\tif false {\n",
  "TestWrite_RefusesOverMaxBytes")
