# A backslash that ends a line joins the next line to it, so an import
# split by backslashes is one statement (debt row 49). Read line by line,
# "from .mm \" followed by an indented "import xx" reads as "import xx"
# alone: an imports edge to the module xx where xx is known, and no edge to
# the module .mm names. Without the join the scan reads every split import
# that way.
m("pylite reads a backslash-continued import line by line", "internal/graph/lang/pylite/scan.go",
  "\t\tjoined := strings.HasSuffix(c, `\\`)\n",
  "\t\tjoined := false\n",
  "TestContImport_ABackslashContinuedImportReadsAsOneStatement")
