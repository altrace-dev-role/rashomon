# RES-2 (PERF-INIT1, D123): the tables taken out of initialisation stay out.
# Each break below keeps every table's contents, so no behaviour test sees
# it; only the init it brings back does (TestRES2f): a holder filled by its
# initialiser, a getter that builds on every call instead of once, and a word
# set computed by a call.
R = "internal/recap/value.go"
V = "internal/verify/pathclass.go"

m("RES-2f a holder is filled at init", R,
  "var noteKeysOnce struct {\n\tsync.Once\n\tv *jsonscan.KeySet\n}\n",
  "var noteKeysOnce = struct {\n\tsync.Once\n\tv *jsonscan.KeySet\n}{v: jsonscan.NewKeySet(\"state\", \"prompt_id\", \"event\", \"pattern\", \"model_text\")}\n",
  "TestRES2f_")
m("RES-2f a getter builds on every call", R,
  "\tnoteKeysOnce.Do(func() {\n\t\tnoteKeysOnce.v = jsonscan.NewKeySet(\"state\", \"prompt_id\", \"event\", \"pattern\", \"model_text\")\n\t})\n\treturn noteKeysOnce.v\n",
  "\treturn jsonscan.NewKeySet(\"state\", \"prompt_id\", \"event\", \"pattern\", \"model_text\")\n",
  "TestRES2f_")
m("RES-2f a word set is computed at init", V,
  '[]string{"dist", "target"}', 'strings.Fields("dist target")', "TestRES2f_")
