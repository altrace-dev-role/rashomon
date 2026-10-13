# The legend that says a listed host was only named. A host under a heading or
# beside a call reads as reached, nothing in the record observed that, and the
# release notes and the installer say the report says so. The --chain line
# lost all three of its guards when the proxy era went; the destinations line
# is new, and keeps the shipped sentences true for the default render.
m("the --chain listing names hosts with no line saying they were only named", "internal/report/text.go",
  "\tif listsAHost(c) {", "\tif false {", "TestText_TheChainListingSays")
m("the --chain listing says its hosts were only named when it lists none", "internal/report/text.go",
  "\tif listsAHost(c) {", "\tif true {", "TestText_TheChainListingSays")
m("the destinations name hosts with no line saying they were only named", "internal/report/text_destinations.go",
  "\tif len(d.DeclaredHosts) > 0 || len(d.SSHHosts) > 0 {", "\tif false {", "TestText_TheDestinationsSay")
m("the destinations say their hosts were only named when they list none", "internal/report/text_destinations.go",
  "\tif len(d.DeclaredHosts) > 0 || len(d.SSHHosts) > 0 {", "\tif true {", "TestText_TheDestinationsSay")
m("the destinations legend forgets ssh hosts", "internal/report/text_destinations.go",
  "\tif len(d.DeclaredHosts) > 0 || len(d.SSHHosts) > 0 {", "\tif len(d.DeclaredHosts) > 0 {",
  "TestText_TheDestinationsSay")
