# A url whose scheme has no default port keeps every port it names
# (internal/baseline/repoid.go, defaultPort and dropDefaultPort): a port is
# dropped only when it equals its own scheme's default. This row gives every
# scheme outside the list ssh's port 22, so web+x://host.example:22 would
# lose its port and name the same repository as the url with no port.
# TestNormalizeOrigin_DropsADefaultPort feeds a scheme outside the list each
# port the list names, and port 0, so a fallthrough to any of them fails it.
m("baseline a scheme outside the default-port list takes ssh's port", "internal/baseline/repoid.go",
  "\t\treturn 990, true\n\t}\n\treturn 0, false\n", "\t\treturn 990, true\n\t}\n\treturn 22, true\n",
  "TestNormalizeOrigin_DropsADefaultPort")
