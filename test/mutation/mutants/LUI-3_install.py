# LUI-3: entry identity is the whole command after the executable, not a
# suffix of it. The old implementation searched h.Command for
# " <subcommand> --install " anywhere, so a longer subcommand sharing a
# shorter one's tail -- "steer post" ends in "post" -- would have read as the
# shorter one's entry. TestLUI3_ pins the exact-match replacement directly;
# TestLUI4_ also goes red, because the same suffix search never finds a
# legacy form that is not itself a bare suffix of the command (e.g.
# "recap-legacy-v1" against " recap --install ").
m("LUI-3 Owner matches a suffix of the command instead of the whole of it",
  "internal/install/install.go",
  "\tforms := append([]string{subcommand(event)}, legacyForms[event]...)\n"
  "\tfor _, h := range g.Hooks {\n"
  "\t\tif id := ownerOf(h.Command, forms); id != \"\" {\n"
  "\t\t\treturn id\n"
  "\t\t}\n"
  "\t}\n"
  "\treturn \"\"\n"
  "}",
  "\tsuffix := \" \" + subcommand(event) + \" \" + Marker + \" \"\n"
  "\tfor _, h := range g.Hooks {\n"
  "\t\tif i := strings.LastIndex(h.Command, suffix); i >= 0 {\n"
  "\t\t\tif id := h.Command[i+len(suffix):]; isInstallID(id) {\n"
  "\t\t\t\treturn id\n"
  "\t\t\t}\n"
  "\t\t}\n"
  "\t}\n"
  "\treturn \"\"\n"
  "}",
  "TestLUI3_")
