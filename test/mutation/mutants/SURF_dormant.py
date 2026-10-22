# The installer places one binary. A second fetch is the loop-0 defect: the
# closed proxy's archive downloaded because a release happened to carry it.
m("install.sh fetches a second archive", "install.sh",
  "say \"  installed ${INSTALL_DIR}/rashomon\"\n",
  "say \"  installed ${INSTALL_DIR}/rashomon\"\n"
  "fetch \"${BASE_URL}/${TAG}/altrace_${VERSION}_${OS}_${ARCH}.tar.gz\" \"$TMP/altrace.tar.gz\" || true\n",
  "TestInstall_")
m("install.sh drops the closing line on what is not observed", "install.sh",
  "say \"Network traffic is not observed. The report lists the hosts each command\"\n",
  "say \"The report lists the hosts each command\"\n",
  "TestInstall_")
# The README's example is pinned to the render. Either side drifting fails it.
m("the README excerpt drifts from the render", "README.md",
  "        agent-a41f (Explore): 3 declarations, 3 executions, 1 Bash\n",
  "        agent-a41f (Explore): 3 declarations, 3 executions\n",
  "TestReadme_")
m("the render drifts from the README excerpt", "internal/report/text.go",
  "\tfmt.Fprintln(b, \"    (these calls do not appear in the main transcript)\")",
  "\tfmt.Fprintln(b, \"    (these calls are not in the main transcript)\")",
  "TestReadme_")
# The usage screen offers no command the binary does not have. env went with
# the proxy era; advertising it again would send a reader to an error.
m("usage advertises the removed env command", "cmd/rashomon/main.go",
  "  rashomon version               print the version\n",
  "  rashomon env [--port N]        print the proxy variables to export\n"
  "  rashomon version               print the version\n",
  "TestUsage_AdvertisesNoDormantProxyPath")
