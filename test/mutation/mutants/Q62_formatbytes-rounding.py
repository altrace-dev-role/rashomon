m("FormatBytes rounds up into the next unit without moving its suffix", "internal/store/capbytes.go",
  "\tif rounded := math.Round(v*10) / 10; rounded >= unit && i < len(units) {\n\t\tv, suffix = v/unit, units[i]\n\t}\n",
  "",
  "TestFormatBytes_RoundingNeverCrossesAUnitWithoutMovingTheSuffix")
IMPORTS["FormatBytes rounds up into the next unit without moving its suffix"] = (
    "internal/store/capbytes.go",
    '\t"io/fs"\n\t"math"\n\t"os"\n',
    '\t"io/fs"\n\t"os"\n')
