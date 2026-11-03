# AG-1 (agent-guard oversight verbs, after #176): the commands that reduce the
# person's oversight refuse an agent session as `steer off|on` does. Each break
# drops one verb's guard and names the test that holds it.
SP = "cmd/rashomon/spec.go"

m("AG-1 spec untrust works in an agent session", SP,
  "\tif s := selfguard.AgentSignal(os.Getenv); s != \"\" && args[0] == \"untrust\" {\n",
  "\tif s := \"\"; s != \"\" && args[0] == \"untrust\" {\n", "TestAGSU1_")

SM = "cmd/rashomon/verbs_summary.go"
# The break removes verbs_summary.go's only use of os and selfguard, so it drops the import
# beside it; without it the mutant does not build, which sweep.py read as red (MASKED-BUILD1).
IMPORTS['AG-1 summary on and off work in an agent session'] = ('cmd/rashomon/verbs_summary.go',
  '\t"os"\n\n\t"github.com/altrace-dev-role/rashomon/internal/selfguard"\n', '\n')
m("AG-1 summary on and off work in an agent session", SM,
  "\tif s := selfguard.AgentSignal(os.Getenv); s != \"\" && len(words) == 1 {\n",
  "\tif s := \"\"; s != \"\" && len(words) == 1 {\n", "TestAGSM1_")

BO = "cmd/rashomon/bodies.go"
# The break removes bodies.go's only use of selfguard, so it drops the import
# beside it; without it the mutant does not build, which sweep.py read as red (MASKED-BUILD1).
IMPORTS['AG-1 bodies on and off work in an agent session'] = ('cmd/rashomon/bodies.go',
  '\t"github.com/altrace-dev-role/rashomon/internal/selfguard"\n', '')
m("AG-1 bodies on and off work in an agent session", BO,
  "\tif s := selfguard.AgentSignal(os.Getenv); s != \"\" {\n",
  "\tif s := \"\"; s != \"\" {\n", "TestAGBO1_")

CU = "cmd/rashomon/consent_unit.go"
# The break removes consent_unit.go's only use of selfguard, so it drops the import
# beside it; without it the mutant does not build, which sweep.py read as red (MASKED-BUILD1).
IMPORTS['AG-1 consent <unit> off works in an agent session'] = ('cmd/rashomon/consent_unit.go',
  '\t"github.com/altrace-dev-role/rashomon/internal/selfguard"\n', '')
m("AG-1 consent <unit> off works in an agent session", CU,
  "\tif s := selfguard.AgentSignal(os.Getenv); off && s != \"\" {\n",
  "\tif s := \"\"; off && s != \"\" {\n", "TestAGCO1_")

PU = "cmd/rashomon/purge.go"
# The break removes purge.go's only use of selfguard, so it drops the import
# beside it; without it the mutant does not build, which sweep.py read as red (MASKED-BUILD1).
IMPORTS['AG-1 purge works in an agent session'] = ('cmd/rashomon/purge.go',
  '\t"github.com/altrace-dev-role/rashomon/internal/selfguard"\n', '')
m("AG-1 purge works in an agent session", PU,
  "\tif s := selfguard.AgentSignal(os.Getenv); s != \"\" {\n",
  "\tif s := \"\"; s != \"\" {\n", "TestAGPU1_")
