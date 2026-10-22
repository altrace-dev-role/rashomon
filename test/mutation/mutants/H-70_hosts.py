# The two breaks below live between shape.Hosts and the record, which is the
# only ground the H-70 acceptance items cover that the unit table does not.
# An audit measured the rest: across twenty mutations of hosts.go the
# acceptance layer caught none the unit layer missed, so these two are what
# the layer is for.
m("H-70 the ssh list is extracted and dropped", "internal/hook/handle.go",
  "\tdecl.Hosts, decl.SSHHosts = shape.Hosts(p.ToolName, p.ToolInput)",
  "\tdecl.Hosts, _ = shape.Hosts(p.ToolName, p.ToolInput)", "TestH70_")
m("H-70 a host of the extractor's own invention", "internal/shape/hosts.go",
  "\tsort.Strings(out)\n\treturn out\n}\n\n// sshDestPrograms",
  "\tout = append(out, \"fabricated.example.com\")\n\tsort.Strings(out)\n\treturn out\n}\n\n// sshDestPrograms",
  "TestH70_SSHDestinationsReachTheRecord")
m("H-70 a destination carrying a password is stripped, not refused", "internal/shape/hosts.go",
  "\t\tif strings.IndexByte(prefix[:at], ':') >= 0 {\n\t\t\treturn \"\", false\n\t\t}",
  "\t\tif false {\n\t\t\treturn \"\", false\n\t\t}", "TestH70_CommandText|TestSSHDestinationRefusesACredential")
m("H-70 a program's arguments run to the end of the line", "internal/shape/hosts.go",
  "\t\tend := i + 1\n\t\tfor end < len(toks) && !(end < len(meta) && meta[end]) {\n\t\t\tend++\n\t\t}",
  "\t\tend := len(toks)", "TestH70_ArgumentsStop|TestSSHHostsAreSorted")
m("H-70 every program is an ssh program", "internal/shape/hosts.go",
  "\t\tif !sshDestPrograms[prog] {\n\t\t\tcontinue\n\t\t}",
  "\t\tif false {\n\t\t\tcontinue\n\t\t}", "TestH70_Local|TestSSHDestinations")
m("H-70 the host list is not sorted", "internal/shape/hosts.go",
  "\tsort.Strings(out)\n\treturn out\n}\n\n// sshDestPrograms", "\treturn out\n}\n\n// sshDestPrograms",
  "TestSSHHostsAreSortedAndDeduped")
m("H-70 the destination is not required to be hostname shaped", "internal/shape/hosts.go",
  "\tif !host.Shaped(hostPart) {\n\t\treturn \"\", false\n\t}",
  "\tif hostPart == \"\" {\n\t\treturn \"\", false\n\t}",
  "TestH70_CommandText|TestSSHDestinationIsHostnameShaped|TestSSHDestinationRefusesWhatIsNotAHost")
m("H-70 one flag table for all four programs", "internal/shape/hosts.go",
  "var valueFlags = map[string]string{\n\t\"ssh\":   \"BbcDEeFIiJLlmOoPpQRSWw\",\n\t\"scp\":   \"cDFiJloPSX\",\n\t\"sftp\":  \"BbcDFiJloPRSsX\",\n\t\"rsync\": \"eBTfM@\",\n}",
  "var valueFlags = map[string]string{\n\t\"ssh\":   \"pPioljJFe\",\n\t\"scp\":   \"pPioljJFe\",\n\t\"sftp\":  \"pPioljJFe\",\n\t\"rsync\": \"pPioljJFe\",\n}",
  "TestH70_Forwarding|TestSSHFlagTables")
m("H-70 ssh hosts come from URL schemes only", "internal/shape/hosts.go",
  "\t\tssh = mergeHosts(ssh, named)", "\t\t_ = named", "TestH70_SSH|TestSSHDestinations")
m("H-70 every non-flag argument is recorded as a host", "internal/shape/hosts.go",
  "\t\tif prog == \"ssh\" || prog == \"sftp\" {\n\t\t\tif h, ok := destinationHost(a, bareHost); ok {\n\t\t\t\tout = append(out, h)\n\t\t\t}\n\t\t\treturn out\n\t\t}\n\t\tif h, ok := destinationHost(a, remoteOnly); ok {\n\t\t\tout = append(out, h)\n\t\t}",
  "\t\tif h, ok := destinationHost(a, bareHost); ok {\n\t\t\tout = append(out, h)\n\t\t}", "TestH70_Local|TestSSHDestinations")
m("H-70 the path is not split off before the user", "internal/shape/hosts.go",
  "\tprefix := tok\n\tif slash := strings.IndexByte(tok, '/'); slash >= 0 {\n\t\tprefix = tok[:slash]\n\t}",
  "\tprefix := tok", "TestSSHFlagTables|TestH70_SSH")
