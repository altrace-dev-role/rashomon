# TRU-2: the consent screen is rendered from the inventory and fits 80
# columns. Each mutant drops or misplaces one thing the screen must carry, or
# lets a line run past the width.
m("TRU-2 a wrapped layer paragraph is dropped", "internal/consent/screen.go",
  "\t\tw.wrap(indent, indent, p.Text)\n", "\t\tif p.Owner != disclose.OwnerMeasure {\n\t\t\tw.wrap(indent, indent, p.Text)\n\t\t}\n",
  "TestTRU2_ScreenNamesEveryEntryCarriesEveryParagraphFitsEighty")
m("TRU-2 a line-for-line paragraph is dropped", "internal/consent/screen.go",
  "\t\t\tfor _, l := range strings.Split(p.Text, \"\\n\") {\n\t\t\t\tw.line(indent + l)",
  "\t\t\tfor _, l := range strings.Split(p.Text, \"\\n\")[1:] {\n\t\t\t\tw.line(indent + l)",
  "TestTRU2_ScreenNamesEveryEntryCarriesEveryParagraphFitsEighty")
m("TRU-2 wrapping lets a line past the width", "internal/consent/screen.go",
  "\troom := func() int { return Width - utf8.RuneCountInString(prefix) }",
  "\troom := func() int { return Width + 8 - utf8.RuneCountInString(prefix) }",
  "TestTRU2_ScreenNamesEveryEntryCarriesEveryParagraphFitsEighty")
m("TRU-2 a long word is not cut at the width", "internal/consent/screen.go",
  "\t\tfor len(cur) == 0 && len(r) > room() {", "\t\tfor false && len(cur) == 0 && len(r) > room() {",
  "TestTRU2_ScreenNamesEveryEntryCarriesEveryParagraphFitsEighty")
m("TRU-2 section 1 drops an entry", "internal/consent/screen.go",
  "\"on: \"+strings.Join(install.Events(), \" \")", "\"on: \"+strings.Join(install.Events()[1:], \" \")",
  "TestTRU2_ScreenNamesEveryEntryCarriesEveryParagraphFitsEighty")
m("TRU-2 section 1 drops the longer time limits", "internal/consent/screen.go",
  "\tif len(others) > 0 {\n\t\tvar parts []string", "\tif false {\n\t\tvar parts []string",
  "TestTRU2_ScreenNamesEveryEntryCarriesEveryParagraphFitsEighty")
m("TRU-2 section 4 loses its heading", "internal/consent/screen.go",
  "\tw.line(\"4. LESSONS  (on this machine only)\")\n", "",
  "TestTRU2_ScreenNamesEveryEntryCarriesEveryParagraphFitsEighty")
m("TRU-2 the screen ends with six commands", "internal/consent/screen.go",
  "\t{\"Remove everything\", \"rashomon purge\"},\n", "",
  "TestTRU2_ScreenNamesEveryEntryCarriesEveryParagraphFitsEighty")
m("TRU-2 a paragraph is printed in the wrong section", "internal/disclose/paragraphs.go",
  "\t\t{Owner: OwnerLearn, Cite: CiteSection4, Text: \"This build learns",
  "\t\t{Owner: OwnerLearn, Cite: CiteSection3, Text: \"This build learns",
  "TestTRU2_ScreenNamesEveryEntryCarriesEveryParagraphFitsEighty")
m("TRU-2 a consent for this scope shows CHANGED SINCE", "internal/consent/screen.go",
  "\tif in.Prior != nil && in.Prior.ScopeVersion < inv.ScopeVersion {",
  "\tif in.Prior != nil && in.Prior.ScopeVersion <= inv.ScopeVersion {",
  "TestTRU2_ScreenVariantsComeFromTheInventory")
m("TRU-2 a missing variant template renders as nothing", "internal/consent/screen.go",
  "\t\tif !ok {\n\t\t\treturn nil, fmt.Errorf(\"consent: the inventory has no %s template\", disclose.CiteChangedSince)",
  "\t\tif false && !ok {\n\t\t\treturn nil, fmt.Errorf(\"consent: the inventory has no %s template\", disclose.CiteChangedSince)",
  "TestTRU2_ScreenVariantsComeFromTheInventory")
m("TRU-2 a machine with no store shows an empty install id", "internal/consent/screen.go",
  "\tif id == \"\" {\n\t\tid = NewInstallPlaceholder", "\tif false {\n\t\tid = NewInstallPlaceholder",
  "TestTRU2_ScreenVariantsComeFromTheInventory")
