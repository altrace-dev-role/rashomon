# MSR-2: the measure package imports no network or subprocess package; the
# driver that starts sessions and runs the oracle is outside the binary.
m("MSR-2 the measure package links os/exec", "internal/measure/transcript.go",
  "import (\n\t\"slices\"\n",
  "import (\n\t_ \"os/exec\"\n\t\"slices\"\n", "TestMSR2_")
