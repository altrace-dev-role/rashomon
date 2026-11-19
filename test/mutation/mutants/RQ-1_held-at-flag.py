# RQ-1: notes held at flag until promoted. heldLevel lowers a
# held pair's nudge to a flag after the stop clamp, unless the operator's
# override names that pair; the override lifts the hold and no other clamp.
# cmd reads RASHOMON_STEER_LEVEL under a forced arm on a lab store only, and
# under both arms, so the off arm decides as the steer arm. Each break is one
# part of that chain.
H = "internal/steer/held.go"
D = "internal/steer/decide.go"
m("RQ-1 a held pair's note reaches the model", H,
  "\t\treturn LevelFlag\n",
  "\t\treturn level\n",
  "TestRule2_HeldPairStaysAtFlagByDefault")
m("RQ-1 the hold is tested before the stop clamp", D,
  "\t\tlevel = heldLevel(ctx, f, level)",
  "\t\tlevel = heldLevel(ctx, f, f.MaxLevel)",
  "TestRule2_HeldPairStaysAtFlagByDefault")
m("RQ-1 the override is ignored", H,
  "HeldAtFlag(p) && ctx.LevelOverride != p {",
  "HeldAtFlag(p) {",
  "TestRule2_OverrideRaisesOnlyItsPair")
m("RQ-1 the override is matched by its kind alone", H,
  "HeldAtFlag(p) && ctx.LevelOverride != p {",
  "HeldAtFlag(p) && ctx.LevelOverride.Kind != p.Kind {",
  "TestRule2_OverrideRaisesOnlyItsPair")
m("RQ-1 the override is matched by its pattern alone", H,
  "HeldAtFlag(p) && ctx.LevelOverride != p {",
  "HeldAtFlag(p) && ctx.LevelOverride.Pattern != p.Pattern {",
  "TestRule2_OverrideRaisesOnlyItsPair")
m("RQ-1 the override lifts the context's clamps", D,
  "\t\t\tif by := clampReason(ctx, p.direct); by != nil {\n",
  "\t\t\tif by := clampReason(ctx, p.direct); by != nil && ctx.LevelOverride == (Pair{}) {\n",
  "TestRule2_OverrideKeepsEveryOtherClamp")
m("RQ-1 the override lifts the flag-only event's clamp", D,
  "\t\tif p.level == LevelNudge && rule == ruleFlagOnly {\n",
  "\t\tif p.level == LevelNudge && rule == ruleFlagOnly && ctx.LevelOverride == (Pair{}) {\n",
  "TestRule2_OverrideKeepsEveryOtherClamp")
m("RQ-1 the override lifts the empty message's clamp", D,
  "\t\tif p.level == LevelNudge && f.Message.Text() == \"\" {\n",
  "\t\tif p.level == LevelNudge && f.Message.Text() == \"\" && ctx.LevelOverride == (Pair{}) {\n",
  "TestRule2_OverrideKeepsEveryOtherClamp")
# The switch: SteerLevel gives a pair only when it is held, contextOf carries
# it to the router, and cmd reads the variable where a forced arm counts.
m("RQ-1 SteerLevel gives a pair no row holds", "internal/hook/steerstep.go",
  "\tif !ok || !steer.HeldAtFlag(p) {\n",
  "\tif !ok {\n",
  "TestSteerLevel_UnknownPairIgnored")
m("RQ-1 the override never reaches the router", "internal/hook/steerstep.go",
  "\t\tLevelOverride:    env.Level,\n",
  "",
  "TestSteerLevel_ReachesTheContext")
C = "cmd/rashomon/steerenv.go"
m("RQ-1 the level is read on a store with no lab marker", C,
  "\tif arm == \"\" {\n",
  "\tif false {\n",
  "TestSteerLevel_HonouredOnlyUnderALabArm")
m("RQ-1 the off arm reads no level and decides apart from the steer arm", C,
  "\tif arm == \"\" {\n",
  "\tif arm != steer.ArmSteer {\n",
  "TestSteerLevel_HonouredOnlyUnderALabArm")
