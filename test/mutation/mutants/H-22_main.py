# The alpha's dormant proxy path. env's guard is the live hazard: without it,
# `eval $(rashomon env)` on a machine with no proxy points the shell's HTTPS at
# a port where nothing listens. Three breaks, because there are three ways to
# lose it -- dropping the check, weakening it to "a status file exists" (which
# the no-proxy test alone cannot tell from the real thing, and which would
# export at an enforcing proxy), and running it before the arguments, which
# makes `env --help` answer "no proxy".
m("H-22 env exports without checking the proxy", "cmd/rashomon/main.go",
  "\tv := posture.Read(posture.DefaultPath())\n\tif !v.Export {",
  "\tv := posture.Read(posture.DefaultPath())\n\tif false {",
  "TestH22_EnvRefuses")
m("H-22 env's check weakened to 'a status file exists'", "cmd/rashomon/main.go",
  "\tv := posture.Read(posture.DefaultPath())\n\tif !v.Export {",
  "\tv := posture.Read(posture.DefaultPath())\n\tif v.File.Product == \"\" {",
  "TestH22_EnvRefusesEveryPostureRunRefuses")
m("H-22 env consults the proxy before reading its own arguments", "cmd/rashomon/main.go",
  "\tport, portGiven, err := envPort(args)\n\tif err != nil {\n\t\treturn err\n\t}\n"
  "\tv := posture.Read(posture.DefaultPath())\n\tif !v.Export {\n"
  "\t\treturn fmt.Errorf(\"env: printing no proxy variables -- %s\", v.Reason)\n\t}\n",
  "\tv := posture.Read(posture.DefaultPath())\n\tif !v.Export {\n"
  "\t\treturn fmt.Errorf(\"env: printing no proxy variables -- %s\", v.Reason)\n\t}\n"
  "\tport, portGiven, err := envPort(args)\n\tif err != nil {\n\t\treturn err\n\t}\n",
  "TestH22_EnvArgumentsAnswerWithoutAProxy|TestH22_EnvRejectsABadPort")
# What env exports once the check passes. The verified address, never the
# default port env used to print as a constant; --port confirms it and cannot
# pick another; --token asks the verified file whether the proxy accepts a tag.
# Re-anchored in loop 2: env exports posture's parsed listener, rebuilt
# (v.Listen.String()), where it exported the file's raw v.File.ListenAddr; and
# --port compares against the parsed port, where it called main.go's own
# listenPort, which is gone.
m("H-22 env exports the default address instead of the verified one", "cmd/rashomon/main.go",
  "\tfor _, kv := range launch.Env(v.Listen.String(), token) {",
  "\tfor _, kv := range launch.Env(\"127.0.0.1:18080\", token) {",
  "TestH22_EnvExportsTheVerifiedAddress")
m("H-22 env accepts a --port the verified listener does not hold", "cmd/rashomon/main.go",
  "\tif portGiven && port != v.Listen.Port {",
  "\tif false && portGiven && port != v.Listen.Port {",
  "TestH22_EnvPortMustNameTheVerifiedListener")

m("H-22 env --token ignores the proxy's capability", "cmd/rashomon/main.go",
  "\tif !v.File.SessionToken {", "\tif false {",
  "TestH22_EnvTokenIsCapabilityGated")
