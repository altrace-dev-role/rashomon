# PF-1, arm off's given set (census #172): arm off decides as steer, so a line
# it withheld (state suppressed, suppressed_by arm_off) is given as a delivered
# one is. Each break stops one reader counting it, so the off arm repeats a path
# or a lesson on every prompt where the steer arm gives it once.
D = "internal/intent/delivered.go"
V = "internal/learn/deliver.go"

m("PF-1 an arm-off line is not a given path", D,
  "text(b, out[dkState]) != string(steer.LineStateDelivered) && text(b, out[dkSuppressedBy]) != string(steer.SuppressedByArmOff) {",
  "text(b, out[dkState]) != string(steer.LineStateDelivered) {", "TestPF1b_ArmOffCountsAsGiven")
m("PF-1 an arm-off line is not a given lesson", V,
  "spanText(b, sp[dkState]) != string(steer.LineStateDelivered) && spanText(b, sp[dkSuppressedBy]) != string(steer.SuppressedByArmOff) {",
  "spanText(b, sp[dkState]) != string(steer.LineStateDelivered) {", "TestLRN17c2_ArmOffGivesTheLessonOnceToo")
