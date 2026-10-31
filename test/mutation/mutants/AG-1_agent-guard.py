# AG-1 (agent-guard oversight verbs, after #176): the commands that reduce the
# person's oversight refuse an agent session as `steer off|on` does. Each break
# drops one verb's guard and names the test that holds it.
SP = "cmd/rashomon/spec.go"

m("AG-1 spec untrust works in an agent session", SP,
  "\tif s := selfguard.AgentSignal(os.Getenv); s != \"\" && args[0] == \"untrust\" {\n",
  "\tif s := \"\"; s != \"\" && args[0] == \"untrust\" {\n", "TestAGSU1_")
