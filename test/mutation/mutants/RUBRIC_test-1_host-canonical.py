# test-1: FuzzHosts holds every recorded host to be a fixed point of
# host.Canonical. Two trailing dots broke that: one was dropped, the rest
# accepted, and a second pass dropped another. They are now refused.
m("test-1  two trailing dots drop one and keep the name", "internal/host/host.go",
  "\t\th = h[:len(h)-1]\n\t\tif strings.HasSuffix(h, \".\") {\n\t\t\treturn \"\", false\n\t\t}\n",
  "\t\th = h[:len(h)-1]\n", "TestRubricTest1_")
