# VER-54: no fact field carries output or edit text. A fact field typed as a
# bare string is the whole regression; the test walks the four fact structs.
m("VER-54 a gate fact carries its id as free text", "internal/verify/finding.go",
  "\tGateID       SlotID\n", "\tGateID       string\n", "TestVER54b_")
