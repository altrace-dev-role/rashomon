# Card 2.5: the field holdout, per session, in the steer line. A held-out
# session's note goes to neither side, neither its text nor the mirror wrapper
# (card VERSION-GATE1: the session's first prints the holdout notice instead);
# its line records delivery holdout with no text and no mirror. Every flag is
# outside the eligible set, which the loop after charge makes the only gate, so
# widening the predicate is live (TestMS5c). Under RASHOMON_HOLDOUT=on the hook
# draws every session at the default share, 1,000 basis points (PRD PF-MS5),
# and an arm refusal is treatment (TestMS5e); RASHOMON_HOLDOUT=off, the
# operator's off, skips the draw (TestMS5f).
D = "internal/steer/decide.go"
H = "internal/hook/steerstep.go"
B = "TestMS5b_HeldOutNoteReachesNeitherSide"
C = "TestMS5c_"
E = "TestMS5e_"
F = "TestMS5f_"

# The held-out note (R1a, R2): user text, model text, the line's delivery.
m("ST-2.5 a held-out note gives the user its text", D,
  "\tp.user, p.model, p.holdout = \"\", \"\", true\n",
  "\tp.user, p.model, p.holdout = p.model, \"\", true\n", B)
m("ST-2.5 held-out note keeps its model text", D,
  "\tp.user, p.model, p.holdout = \"\", \"\", true\n",
  "\tp.user, p.model, p.holdout = \"\", p.model, true\n", B)
m("ST-2.5 held-out note keeps the mirror wrapper", D,
  "\tp.user, p.model, p.holdout = \"\", \"\", true\n",
  "\tp.user, p.model, p.holdout = p.user, \"\", true\n", B)
m("ST-2.5 a held-out line records a mirror", D,
  "\tif p.state == LineStateDelivered && !p.holdout {\n", "\tif p.state == LineStateDelivered {\n", B)
m("ST-2.5 held-out line keeps the derived delivery", D,
  "\t\td := DeliveryHoldout // recorded by the draw, never derived (tech 5.2.4 E)\n\t\tl.Delivery = &d\n",
  "", B)

# The eligible set (R3): a delivered note only.
m("ST-2.5 holdout predicate widened to flags", D,
  "\treturn p.state == LineStateDelivered && p.level == LevelNudge\n",
  "\treturn p.state == LineStateDelivered\n", C)

# The draw (R1d): made on every call under the on switch, at the default share.
m("ST-2.5 the draw is skipped, every session treated", H,
  "\t\tHoldout:    p.heldOut(env),\n", "\t\tHoldout:    false,\n", E)
m("ST-2.5 the on switch's share is 0, so every session is treated", H,
  "const holdoutBP = 1000\n", "const holdoutBP = 0\n", E)
m("ST-2.5 the env off is ignored", H,
  "\tif env.HoldoutOff || env.Arm != \"\" {\n\t\treturn false\n\t}\n", "\tif env.Arm != \"\" {\n\t\treturn false\n\t}\n", F)
