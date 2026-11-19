# A from-import with leading dots names a module relative to the file's
# package. A from pattern that takes no leading dots reads no relative
# import at all, so the file imports nothing and a call through the name
# resolves to nothing.
m("pylite reads no relative from-import", "internal/graph/lang/pylite/scan.go",
  "\t\tpatterns.from = regexp.MustCompile(`^from\\s+(\\.+(?:[A-Za-z_][A-Za-z0-9_.]*)?|[A-Za-z_][A-Za-z0-9_.]*)\\s+import\\s+(.+)$`)\n",
  "\t\tpatterns.from = regexp.MustCompile(`^from\\s+([A-Za-z_][A-Za-z0-9_.]*)\\s+import\\s+(.+)$`)\n",
  "TestRelImport_EachFormResolvesAgainstThePackage")

# Inside a package's __init__.py the package is the file's own module, so
# one dot names a module inside it. Taking the module less its last part,
# as for any other file, goes one package too high: from the top package's
# __init__.py the import reads as past the top and is dropped.
m("pylite takes an __init__.py's package as its parent", "internal/graph/lang/pylite/pylite.go",
  "\tf.resolveRelative(mod, strings.HasSuffix(\"/\"+rel, \"/__init__.py\"))\n",
  "\tf.resolveRelative(mod, false)\n",
  "TestRelImport_EachFormResolvesAgainstThePackage")

# As many dots as the package has parts is the top package itself, an
# import Python accepts. Refusing at that count drops every one-dot import
# from a module of a top-level package.
m("pylite refuses a relative import at the top package", "internal/graph/lang/pylite/pylite.go",
  "\tif dots > len(parts) {\n",
  "\tif dots >= len(parts) {\n",
  "TestRelImport_EachFormResolvesAgainstThePackage")

# More dots than the package has parts is an import Python refuses, so no
# edge is written for it. Stopping at the root instead reads the rest as an
# absolute module and writes an edge to an unrelated module of that name.
m("pylite resolves an import past the top package from the root", "internal/graph/lang/pylite/pylite.go",
  "\tif dots > len(parts) {\n\t\treturn \"\", false\n\t}\n",
  "\tif dots > len(parts) {\n\t\treturn rest, true\n\t}\n",
  "TestRelImport_EachFormResolvesAgainstThePackage")

# Dots alone, as in "from . import x", name the package itself. Joining the
# empty rest on a dot names a module with a trailing dot that no file
# declares.
m("pylite joins an empty module on a dot", "internal/graph/lang/pylite/pylite.go",
  "\tif rest == \"\" {\n\t\treturn base, true\n\t}\n",
  "",
  "TestRelImport_EachFormResolvesAgainstThePackage")

# Each file declares its own module's id, the id an import of that module
# ends at, so a reader walking from an importer reaches the file. Without
# the declaration an import edge ends at an id no file holds, and the
# importers of a module do not reach its file.
m("pylite writes no module id for a file", "internal/graph/lang/pylite/pylite.go",
  "\t// The file declares its module's id, the id an import of the module ends\n\t// at, so an importer reaches the file at depth 2. No node carries it.\n\trecs = append(recs, graph.Record{Edge: &graph.Edge{Src: file, Pred: graph.Declares, Dst: graph.SymID(Lang, u.Path, \"mod\", mod),\n\t\tTier: graph.Algorithmic, Conf: defConf, Rev: rev, Prov: algo}})\n",
  "",
  "TestRelImport_EachFileDeclaresItsModule")
