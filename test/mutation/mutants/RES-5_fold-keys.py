# RES-5: payload keys are matched as the old struct decode matched its field
# names, in any case. An exact-only fold KeySet, a streamed path that ignores
# Fold, or an intake built on an exact KeySet is the regression.
m("RES-5 a fold KeySet matches only exactly", "internal/jsonscan/jsonscan.go",
  "\tif k.fold {\n\t\tfor i, want := range k.sorted {\n",
  "\tif k.fold && false {\n\t\tfor i, want := range k.sorted {\n",
  "TestRES5_")
m("RES-5 a streamed path ignores Fold", "internal/jsonscan/stream.go",
  "\treturn p.Elems[d] == elem || (p.Fold && strings.EqualFold(p.Elems[d], elem))\n",
  "\treturn p.Elems[d] == elem || (p.Fold && strings.EqualFold(p.Elems[d], elem) && false)\n",
  "TestRES5_")
m("RES-5 intake matches payload keys exactly", "internal/hookio/payload.go",
  "var payloadKeys = jsonscan.NewFoldKeySet(KeyNames()...)\n",
  "var payloadKeys = jsonscan.NewKeySet(KeyNames()...)\n",
  "TestRES5_")
m("RES-5 run_in_background is matched exactly", "internal/shape/verb.go",
  "var runInBackgroundKey = jsonscan.NewFoldKeySet(\"run_in_background\")\n",
  "var runInBackgroundKey = jsonscan.NewKeySet(\"run_in_background\")\n",
  "TestRES5_")
m("RES-5 the background keys are matched exactly", "internal/hook/post.go",
  "var backgroundKeys = jsonscan.NewFoldKeySet(\"backgroundTaskId\", \"backgroundedByUser\")\n",
  "var backgroundKeys = jsonscan.NewKeySet(\"backgroundTaskId\", \"backgroundedByUser\")\n",
  "TestRES5_")
