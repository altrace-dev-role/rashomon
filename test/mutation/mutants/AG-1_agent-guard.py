# AG-1 (agent-guard oversight verbs, after #176): the commands that reduce the
# person's oversight refuse an agent session as `steer off|on` does. Each break
# drops one verb's guard and names the test that holds it.
SP = "cmd/rashomon/spec.go"

m("AG-1 spec untrust works in an agent session", SP,
  "\tif s := selfguard.AgentSignal(os.Getenv); s != \"\" && args[0] == \"untrust\" {\n",
  "\tif s := \"\"; s != \"\" && args[0] == \"untrust\" {\n", "TestAGSU1_")

SM = "cmd/rashomon/verbs_summary.go"
m("AG-1 summary on and off work in an agent session", SM,
  "\tif s := selfguard.AgentSignal(os.Getenv); s != \"\" && len(words) == 1 {\n",
  "\tif s := \"\"; s != \"\" && len(words) == 1 {\n", "TestAGSM1_")

BO = "cmd/rashomon/bodies.go"
m("AG-1 bodies on and off work in an agent session", BO,
  "\tif s := selfguard.AgentSignal(os.Getenv); s != \"\" {\n",
  "\tif s := \"\"; s != \"\" {\n", "TestAGBO1_")

CU = "cmd/rashomon/consent_unit.go"
m("AG-1 consent <unit> off works in an agent session", CU,
  "\tif s := selfguard.AgentSignal(os.Getenv); off && s != \"\" {\n",
  "\tif s := \"\"; off && s != \"\" {\n", "TestAGCO1_")
