# B39 -- a field the shape and the execution record both carry is copied at
# every site that copies the others. The two copies of the masked triple --
# the execution record's from the shape of the input that ran (hook/post.go),
# and the declaration's from the as-ran record (report MaskingAsRan) -- are
# checked by reflection over every field of both structs, the one copied
# from and the one copied into (recordtest.AssertCopy), so a field dropped
# from a copy, swapped with another, rewritten where it should be left, or
# added to either struct and left out of the lists fails by name.
m("B39 the execution record drops the shape's runner kind", "internal/hook/post.go",
  "\t\trec.StatusMasked, rec.RunnerDigest, rec.RunnerKind = s.StatusMasked, s.RunnerDigest, s.RunnerKind\n",
  "\t\trec.StatusMasked, rec.RunnerDigest = s.StatusMasked, s.RunnerDigest\n",
  "TestB39_PostCopiesTheShapesMaskedTriple")
m("B39 the as-ran masking drops the record's runner kind", "internal/report/timeline.go",
  "\t\td.Shape.StatusMasked, d.Shape.RunnerDigest, d.Shape.RunnerKind = rec.StatusMasked, rec.RunnerDigest, rec.RunnerKind\n",
  "\t\td.Shape.StatusMasked, d.Shape.RunnerDigest = rec.StatusMasked, rec.RunnerDigest\n",
  "TestB39_MaskingAsRanCopiesTheTriple")
m("B39 the as-ran masking swaps the runner digest and kind", "internal/report/timeline.go",
  "\t\td.Shape.StatusMasked, d.Shape.RunnerDigest, d.Shape.RunnerKind = rec.StatusMasked, rec.RunnerDigest, rec.RunnerKind\n",
  "\t\td.Shape.StatusMasked, d.Shape.RunnerDigest, d.Shape.RunnerKind = rec.StatusMasked, rec.RunnerKind, rec.RunnerDigest\n",
  "TestB39_MaskingAsRanCopiesTheTriple")
m("B39 the as-ran masking rebuilds the shape from the record alone", "internal/report/timeline.go",
  "\t\td.Shape.StatusMasked, d.Shape.RunnerDigest, d.Shape.RunnerKind = rec.StatusMasked, rec.RunnerDigest, rec.RunnerKind\n",
  "\t\td.Shape = shape.Shape{StatusMasked: rec.StatusMasked, RunnerDigest: rec.RunnerDigest, RunnerKind: rec.RunnerKind}\n",
  "TestB39_MaskingAsRanCopiesTheTriple")
m("B39 a withheld field the copy rewrites passes", "internal/store/recordtest/recordtest.go",
  "\t\tif av.IsValid() && !equal(bv, av) {\n",
  "\t\tif false && av.IsValid() && !equal(bv, av) {\n",
  "TestB39_")
