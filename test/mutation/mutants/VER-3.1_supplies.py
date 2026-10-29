# VER-3.1 supplies: a gate's evidence kinds (D117, card 3.1 (a)), read by 3.3.
# The list is closed and each kind appears once; TestSP14 holds both rules.
P = "internal/spec/parse.go"
T = "TestSP14_SuppliesIsClosed"

m("VER-3.1 supplies is an open list", P, "out := list(r, n, r.oneOf(supplyKinds))", "out := list(r, n, r.plain)", T)
m("VER-3.1 a supplies kind may appear twice", P, "if slices.Contains(out[:i], out[i]) {", "if false {", T)
m("VER-3.1 supplies is accepted and not read", P, '\tg.Supplies = field(r, f, "supplies", false, r.supplies)\n', "", T)
m("VER-3.1 supplies is not a gate key", P, '"empty_run_line", "exit_codes", "supplies"}', '"empty_run_line", "exit_codes"}', T)
