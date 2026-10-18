# forget --host records its digest under the CANONICAL spelling, never the
# one a declaration happens to hold. declaredHostsFiltered and
# sshDeclaredFiltered canonicalised a host for the output set but asked
# forgotten() about the stored spelling, so a declaration recorded before
# host.Canonical existed -- or an ssh host sshDeclaredFiltered never
# canonicalised at all -- did not match the digest and escaped the check,
# reappearing in destinations.declared_hosts / destinations.ssh_hosts on
# every later render of the run that still held it, even after a forget of
# its canonical name reported success.
m("RUBRIC lld-11b declaredHostsFiltered asks forgotten about the stored spelling", "internal/report/destinations.go",
  "// declaration -- 167838211 where a forget named 10.1.2.3 --\n"
  "\t\t\t// escape the check and reappear in every later render of the\n"
  "\t\t\t// same old run.\n"
  "\t\t\tcanon, ok := host.Canonical(h)\n\t\t\tif !ok || !host.Shaped(canon) {\n\t\t\t\tcontinue\n\t\t\t}\n"
  "\t\t\tif forgotten != nil && forgotten(canon, d.RecordedAtMS) {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tset[canon] = true\n",
  "// declaration -- 167838211 where a forget named 10.1.2.3 --\n"
  "\t\t\t// escape the check and reappear in every later render of the\n"
  "\t\t\t// same old run.\n"
  "\t\t\tcanon, ok := host.Canonical(h)\n\t\t\tif !ok || !host.Shaped(canon) {\n\t\t\t\tcontinue\n\t\t\t}\n"
  "\t\t\tif forgotten != nil && forgotten(h, d.RecordedAtMS) {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tset[canon] = true\n",
  "TestSeam_ForgetFindsADeclarationStoredInALegacySpelling")
m("RUBRIC lld-11b sshDeclaredFiltered asks forgotten about the stored spelling", "internal/report/destinations.go",
  "for _, h := range d.SSHHosts {\n"
  "\t\t\tcanon, ok := host.CanonicalName(h)\n\t\t\tif !ok || !host.Shaped(canon) {\n\t\t\t\tcontinue\n\t\t\t}\n"
  "\t\t\tif forgotten != nil && forgotten(canon, d.RecordedAtMS) {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tset[canon] = true\n",
  "for _, h := range d.SSHHosts {\n"
  "\t\t\tcanon, ok := host.CanonicalName(h)\n\t\t\tif !ok || !host.Shaped(canon) {\n\t\t\t\tcontinue\n\t\t\t}\n"
  "\t\t\tif forgotten != nil && forgotten(h, d.RecordedAtMS) {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tset[canon] = true\n",
  "TestSeam_ForgetFindsAnSSHDeclarationStoredInALegacySpelling")
