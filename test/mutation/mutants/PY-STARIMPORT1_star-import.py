# A star import binds the public names of its module, which the scan cannot
# list from the importing file, and no name called *. Read as one imported
# name called *, it writes a references edge to a const named by the star,
# and a second star import in the file replaces the first, whose module then
# gets no imports edge.
m("pylite reads a star import as a name called *", "internal/graph/lang/pylite/scan.go",
  "\t\tif part == \"*\" {\n\t\t\timports[\"*\"+module] = importRef{module: module}\n\t\t\tcontinue\n\t\t}\n",
  "",
  "TestStarImport_ImportsItsModuleAndBindsNoName")

# Each star import is kept under its own module. Kept under the star alone,
# a second star import in the file replaces the first, and the first
# module's imports edge is lost.
m("pylite keeps one star import per file", "internal/graph/lang/pylite/scan.go",
  "\t\t\timports[\"*\"+module] = importRef{module: module}\n",
  "\t\t\timports[\"*\"] = importRef{module: module}\n",
  "TestStarImport_ImportsItsModuleAndBindsNoName")

# A star import is kept under a key no name can take. Kept under its module
# alone, a star import of a top-level module named like a name the file
# binds (from mm import * beside from x import mm) and that name share one
# key, so the later of the two replaces the other: one imports edge is lost
# either way, and with the star import second the references edge to x.mm
# is lost too.
m("pylite keeps a star import under its module's name", "internal/graph/lang/pylite/scan.go",
  "\t\t\timports[\"*\"+module] = importRef{module: module}\n",
  "\t\t\timports[module] = importRef{module: module}\n",
  "TestStarImport_ImportsItsModuleAndBindsNoName")
