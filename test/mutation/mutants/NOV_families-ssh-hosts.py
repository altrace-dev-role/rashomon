# A family whose calls only ever named ssh hosts -- `git clone
# git@github.com:a/b` and nothing else -- must not read as having declared
# none. The https-only count was a proxy-era carve-out that outlived the
# proxy.
m("families counts only https hosts, dropping ssh ones", "internal/report/families.go",
  "\t\tfor _, h := range d.SSHHosts {\n\t\t\tif forgotten(h, d.RecordedAtMS) {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\ta.hosts[h] = true\n\t\t}\n",
  "",
  "TestFamilies_SSHHostsCountToo")
