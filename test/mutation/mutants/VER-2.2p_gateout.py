# The classifier's table, bounds and precedence are observable record rules.
# Every change below is expected to be killed by its named GO acceptance row.
m("VER-2.2p Go pass pattern removed", "internal/gateout/tables.go",
  "pass: compile(`^ok\\s`)", "pass: nil", "TestGO1_GoTestOkIsSummaryPass|TestGO12_|TestGO13_")
m("VER-2.2p Go fail pattern removed", "internal/gateout/tables.go",
  "fail: compile(`^FAIL\\s`)", "fail: nil", "TestGO11_|TestGO12_")
m("VER-2.2p Go fail line pattern removed", "internal/gateout/tables.go",
  "failLine: compile(`^--- FAIL`)", "failLine: nil", "TestGO2_")
m("VER-2.2p Go no-test-files pattern removed", "internal/gateout/tables.go",
  "compile(`\\[no test files\\]`, `\\[no tests to run\\]`)",
  "compile(`\\[no tests to run\\]`)", "TestGO3_")
m("VER-2.2p Go no-tests-to-run pattern removed", "internal/gateout/tables.go",
  "compile(`\\[no test files\\]`, `\\[no tests to run\\]`)",
  "compile(`\\[no test files\\]`)", "TestGO14_")
m("VER-2.2p pytest pass pattern removed", "internal/gateout/tables.go",
  "pass: compile(`\\d+ passed`)", "pass: nil", "TestGO4_")
m("VER-2.2p pytest fail pattern removed", "internal/gateout/tables.go",
  "fail: compile(`\\d+ (failed|error)`)", "fail: nil", "TestGO4_")
m("VER-2.2p pytest empty pattern removed", "internal/gateout/tables.go",
  "empty: compile(`no tests ran`)", "empty: nil", "TestGO4_")
m("VER-2.2p Jest pass pattern removed", "internal/gateout/tables.go",
  "pass: compile(`^Tests:.*\\d+ passed`)", "pass: nil", "TestGO5_|TestGO7_")
m("VER-2.2p Jest fail pattern removed", "internal/gateout/tables.go",
  "fail: compile(`^Tests:.*\\d+ failed`)", "fail: nil", "TestGO5_")
m("VER-2.2p Vitest pass pattern removed", "internal/gateout/tables.go",
  "pass: compile(`^Tests\\s+.*\\d+ passed`)", "pass: nil", "TestGO7_")
m("VER-2.2p Vitest fail pattern removed", "internal/gateout/tables.go",
  "fail: compile(`^Tests\\s+.*\\d+ failed`)", "fail: nil", "TestGO7_")
m("VER-2.2p Cargo pass pattern removed", "internal/gateout/tables.go",
  "pass: compile(`^test result: ok`)", "pass: nil", "TestGO6_")
m("VER-2.2p Cargo fail pattern removed", "internal/gateout/tables.go",
  "fail: compile(`^test result: FAILED`)", "fail: nil", "TestGO6_")

m("VER-2.2p Go pass and fail exchanged", "internal/gateout/tables.go",
  "pass: compile(`^ok\\s`), fail: compile(`^FAIL\\s`)",
  "pass: compile(`^FAIL\\s`), fail: compile(`^ok\\s`)", "TestGO1_|TestGO11_|TestGO12_")
m("VER-2.2p pytest pass and fail exchanged", "internal/gateout/tables.go",
  "pass: compile(`\\d+ passed`), fail: compile(`\\d+ (failed|error)`)",
  "pass: compile(`\\d+ (failed|error)`), fail: compile(`\\d+ passed`)", "TestGO4_")
m("VER-2.2p Jest pass and fail exchanged", "internal/gateout/tables.go",
  "pass: compile(`^Tests:.*\\d+ passed`), fail: compile(`^Tests:.*\\d+ failed`)",
  "pass: compile(`^Tests:.*\\d+ failed`), fail: compile(`^Tests:.*\\d+ passed`)", "TestGO5_")
m("VER-2.2p Vitest pass and fail exchanged", "internal/gateout/tables.go",
  "pass: compile(`^Tests\\s+.*\\d+ passed`), fail: compile(`^Tests\\s+.*\\d+ failed`)",
  "pass: compile(`^Tests\\s+.*\\d+ failed`), fail: compile(`^Tests\\s+.*\\d+ passed`)", "TestGO7_")
m("VER-2.2p Cargo pass and fail exchanged", "internal/gateout/tables.go",
  "pass: compile(`^test result: ok`), fail: compile(`^test result: FAILED`)",
  "pass: compile(`^test result: FAILED`), fail: compile(`^test result: ok`)", "TestGO6_")

m("VER-2.2p size cap removed", "internal/gateout/gateout.go",
  "if len(text) > MaxScanBytes {", "if false {", "TestGO9_")
m("VER-2.2p size cap off by one", "internal/gateout/gateout.go",
  "if len(text) > MaxScanBytes {", "if len(text) >= MaxScanBytes {", "TestGO9_")
m("VER-2.2p fail loses to pass", "internal/gateout/gateout.go",
  "if fail {\n\t\treturn store.GateOutput{Kind: \"summary_fail\"",
  "if fail && !pass {\n\t\treturn store.GateOutput{Kind: \"summary_fail\"", "TestGO12_")
m("VER-2.2p pass loses to empty", "internal/gateout/gateout.go",
  "if pass {\n\t\treturn store.GateOutput{Kind: \"summary_pass\"",
  "if pass && !empty {\n\t\treturn store.GateOutput{Kind: \"summary_pass\"", "TestGO13_")
m("VER-2.2p fail line loses to pass", "internal/gateout/gateout.go",
  "if failLine {\n\t\treturn store.GateOutput{Kind: \"fail_line\"",
  "if failLine && !pass {\n\t\treturn store.GateOutput{Kind: \"fail_line\"", "TestGO21_")
m("VER-2.2p empty is not recognized before pass", "internal/gateout/gateout.go",
  "if matches(row.empty, line) {", "if false && matches(row.empty, line) {", "TestGO14_")
m("VER-2.2p delegate does not scan other rows", "internal/gateout/gateout.go",
  "if delegate {\n\t\trow = nil\n\t}", "if delegate {\n\t\trow = findProgram(program)\n\t}", "TestGO7_")
m("VER-2.2p build output is scanned", "internal/gateout/gateout.go",
  "if kind == \"build\" {\n\t\treturn store.GateOutput{Kind: \"no_summary\", Parser: program}\n\t}",
  "if false {\n\t\treturn store.GateOutput{Kind: \"no_summary\", Parser: program}\n\t}", "TestGO16_")
