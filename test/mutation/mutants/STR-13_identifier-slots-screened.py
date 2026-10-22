# STR-13: TaskID, Model, AgentType, RuleID, Program (and Gate through it)
# and Mechanism run the secret deny list and the entropy test after their
# charset, so a token that fits a slot's charset is still refused. A slot
# skipping the screen, or the screen losing either half, is the regression.
m("STR-13 Program skips the secret screen", "internal/steer/render/slot/slot.go",
  "\treturn screened(\"program\", string(p))\n", "\treturn nil\n", "TestSTR13i_")
m("STR-13 TaskID skips the secret screen", "internal/steer/render/slot/slot.go",
  "\treturn screened(\"task id\", string(t))\n", "\treturn nil\n", "TestSTR13i_")
m("STR-13 AgentType skips the secret screen", "internal/steer/render/slot/slot.go",
  "\treturn screened(\"agent type\", string(a))\n", "\treturn nil\n", "TestSTR13i_")
m("STR-13 RuleID skips the secret screen", "internal/steer/render/slot/slot.go",
  "\treturn screened(\"rule id\", string(r))\n", "\treturn nil\n", "TestSTR13i_")
m("STR-13 Model skips the secret screen", "internal/steer/render/slot/slot.go",
  "\treturn screened(\"model\", string(m))\n", "\treturn nil\n", "TestSTR13i_")
m("STR-13 Mechanism skips the entropy test", "internal/steer/render/slot/slot.go",
  "\treturn screened(\"mechanism\", s)\n", "\treturn denied(\"mechanism\", s)\n", "TestSTR13i_")
m("STR-13 the identifier screen skips the deny list", "internal/steer/render/slot/secret.go",
  "\tif err := denied(slot, s); err != nil {\n\t\treturn err\n\t}\n\tif highEntropy(s) {\n",
  "\tif highEntropy(s) {\n", "TestSTR13i_")
m("STR-13 the identifier screen skips the entropy test", "internal/steer/render/slot/secret.go",
  "\tif highEntropy(s) {\n\t\treturn fmt.Errorf(\"%w: %s fails the entropy test\", ErrSecretShaped, slot)\n",
  "\tif false {\n\t\treturn fmt.Errorf(\"%w: %s fails the entropy test\", ErrSecretShaped, slot)\n", "TestSTR13i_")
