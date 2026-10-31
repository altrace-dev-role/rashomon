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

PU = "cmd/rashomon/purge.go"
m("AG-1 purge works in an agent session", PU,
  "\tif s := selfguard.AgentSignal(os.Getenv); s != \"\" {\n",
  "\tif s := \"\"; s != \"\" {\n", "TestAGPU1_")

# SELFGUARD-ALL1 (R31): detach and pause refuse too, every guard returns
# selfguard.Refusal, and the stop skills hand the person the command.
DT = "cmd/rashomon/detach.go"
m("AG-1 detach works in an agent session", DT,
  "\tif s := selfguard.AgentSignal(os.Getenv); s != \"\" {\n",
  "\tif s := \"\"; s != \"\" {\n", "TestAGDP1_")

MN = "cmd/rashomon/main.go"
m("AG-1 pause works in an agent session", MN,
  "\tif s := selfguard.AgentSignal(os.Getenv); s != \"\" {\n",
  "\tif s := \"\"; s != \"\" {\n", "TestAGDP1_")
m("AG-1 pause writes its own refusal text", MN,
  "\t\treturn selfguard.Refusal(s, \"pause stops the recording of your sessions until you resume it\", \"pause\", \"written\")\n",
  "\t\treturn fmt.Errorf(\"pause stops the recording of your sessions until you resume it, so only you can run it, in %s: `rashomon pause`. Nothing was written. (%s is set here: Claude Code sets it for every command it runs, a `!` line included.)\", selfguard.Place, s)\n",
  "TestAGRF1_")

# #177 review T1: the root's file list alone missed a step before the refusal
# that changes settings.json, as purge's detach does.
m("AG-1 purge changes settings.json before it refuses", PU,
  "\tif s := selfguard.AgentSignal(os.Getenv); s != \"\" {\n",
  "\tif s := selfguard.AgentSignal(os.Getenv); s != \"\" && os.Remove(filepath.Join(os.Getenv(\"CLAUDE_CONFIG_DIR\"), \"settings.json\")) == nil {\n",
  "TestAGPU1_")

SG = "internal/selfguard/selfguard.go"
m("AG-1 a refusal sends the person to their own terminal", SG,
  "const Place = \"a terminal outside Claude Code\"\n",
  "const Place = \"your own terminal\"\n", "TestAGDP1_")
m("AG-1 a refusal leads with the signal", SG,
  "\treturn fmt.Errorf(\"%s, so only you can run it, in %s: `rashomon %s`. Nothing was %s. (%s is set here: Claude Code sets it for every command it runs, a `!` line included.)\",\n\t\twhy, Place, command, nothing, signal)\n",
  "\treturn fmt.Errorf(\"%s is set, so this is an agent session; %s, so only you can run it, in %s: `rashomon %s`. Nothing was %s.\",\n\t\tsignal, why, Place, command, nothing)\n",
  "TestAGDP1_")

