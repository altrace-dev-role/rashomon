# LANG-REG1c, methods keyed by their class: a def in a class body is a
# method, symkind method, its qualified name the module, the classes and its
# own name, so it shares no id with a module def of its name; a self.name()
# or cls.name() call in a method resolves in the caller's own class and
# nowhere else; a bare name in a method resolves among the module's defs
# only; a name imported from a known unit is guessed a class by its CapWords
# case; a def nested in a def's body is that body; the reader is pylite/3. A
# class is no node yet. One row per Break clause of the tests in
# internal/graph/lang/pylite/scope_test.go and one per other place a rule is
# written, each killed by the test it names. Written, not run: for the end
# sweep.
pylite = "internal/graph/lang/pylite/pylite.go"
names = "internal/graph/lang/pylite/names.go"
scan = "internal/graph/lang/pylite/scan.go"

m("LANG-REG1c a def nested in a body is tested as a def first", scan,
  '\t\tif cur != "" && indent > curIndent {\n',
  '\t\tif cur != "" && indent > curIndent && pats().def.FindStringIndex(line) == nil {\n',
  "TestLANGREG1_ADefInsideADefIsItsBody")

m("LANG-REG1c a def in a class body has no class", scan,
  "\t\t\td.class = outer\n", '\t\t\td.class = ""\n',
  "TestLANGREG1_AMethodIsKeyedByItsClass")
m("LANG-REG1c a method is keyed as a module def", names,
  '\t\tif d.class != "" {\n\t\t\tbyKey[d.key()]', "\t\tif false {\n\t\t\tbyKey[d.key()]",
  "TestLANGREG1_AMethodIsKeyedByItsClass")
m("LANG-REG1c a method's node says symkind func", pylite,
  '\t\t\tkind = "method"\n', '\t\t\tkind = "func"\n',
  "TestLANGREG1_AMethodIsKeyedByItsClass")
m("LANG-REG1c a class ends at the close of bases that span lines", scan,
  "len(m[0])-1); ok {\n\t\t\t\t\ti = end\n", "len(m[0])-1); ok {\n\t\t\t\t\t_ = end\n",
  "TestLANGREG1_AMethodIsKeyedByItsClass")

m("LANG-REG1c a self call is left unresolved", names,
  "\t\t\treturn rest, true\n", "\t\t\treturn rest, false\n",
  "TestLANGREG1_ASelfCallResolvesInItsOwnClass")
m("LANG-REG1c a self call is looked up in the outermost class", names,
  'strings.LastIndex(c.caller, ".")', 'strings.Index(c.caller, ".")',
  "TestLANGREG1_ASelfCallResolvesInItsOwnClass")
m("LANG-REG1c a self call its class does not define falls back to the module's defs", names,
  '\t\t\tid, found := byKey[c.caller[:dot]+"."+name]\n\t\t\treturn id, found\n',
  '\t\t\tif id, found := byKey[c.caller[:dot]+"."+name]; found {\n\t\t\t\treturn id, found\n'
  '\t\t\t}\n\t\t\treturn r.callee(name, local, imports)\n',
  "TestLANGREG1_ASelfCallResolvesInItsOwnClass")
m("LANG-REG1c a cls call is no self call", names,
  '[]string{"self.", "cls."}', '[]string{"self."}',
  "TestLANGREG1_ASelfCallResolvesInItsOwnClass")

m("LANG-REG1c a method is among the module's names", names,
  "\t\t\tcontinue\n", "",
  "TestLANGREG1_ABareNameIsAModuleDef")
m("LANG-REG1c a def at a class's indent stays in the class", scan,
  "indent <= classes[len(classes)-1].indent", "indent < classes[len(classes)-1].indent",
  "TestLANGREG1_ABareNameIsAModuleDef")

m("LANG-REG1c every name is CapWords", names,
  "return s != \"\" && s[0] >= 'A' && s[0] <= 'Z' && strings.ToUpper(s) != s", 'return s != ""',
  "TestLANGREG1_ACapWordsNameGuessesAClass")
m("LANG-REG1c an imported CapWords name is referenced as a function", pylite,
  "} else if capWords(im.attr) {\n", "} else if false {\n",
  "TestLANGREG1_ACapWordsNameGuessesAClass")
m("LANG-REG1c a call of an imported CapWords name is keyed a function", pylite,
  "\tif capWords(attr) {\n", "\tif false {\n",
  "TestLANGREG1_ACapWordsNameGuessesAClass")

m("LANG-REG1c the reader stays pylite/1", pylite,
  '\tprovVersion   = "3"\n', '\tprovVersion   = "1"\n',
  "TestLANGREG1_TheReaderIsPyliteThree")
