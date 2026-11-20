# A scheme outside the default-port list, web+x being the one fed, keeps
# each port the list names, and port 0 (internal/baseline/repoid.go,
# defaultPort and dropDefaultPort). The first row gives
# every scheme outside the list ssh's port 22, so web+x://host.example:22
# would lose its port and name the same repository as the url with no port.
# The second keeps the 0 the switch returns for such a scheme but says true,
# so web+x://host.example:0 would lose its port the same way.
# TestNormalizeOrigin_DropsADefaultPort feeds a scheme outside the list each
# port the list names, and port 0, so a fallthrough to any of them fails it.
m("baseline a scheme outside the default-port list takes ssh's port", "internal/baseline/repoid.go",
  "\t\treturn 990, true\n\t}\n\treturn 0, false\n", "\t\treturn 990, true\n\t}\n\treturn 22, true\n",
  "TestNormalizeOrigin_DropsADefaultPort")
m("baseline a scheme outside the default-port list takes port 0 as its default", "internal/baseline/repoid.go",
  "\t\treturn 990, true\n\t}\n\treturn 0, false\n", "\t\treturn 990, true\n\t}\n\treturn 0, true\n",
  "TestNormalizeOrigin_DropsADefaultPort")
