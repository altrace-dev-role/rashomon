m("the surface check matches nothing", "test/acceptance/dormant_surface_test.go",
  "regexp.MustCompile(`(?i)\\brashomon (run|env)\\b|proxy-store`)", "regexp.MustCompile(`^$x`)",
  "TestSurface_|TestInstall_")
# Re-anchored after the README was shortened: the install block lost its
# comment column, so the dormant command is offered beside the bare `rashomon watch`.
m("the README offers a dormant command", "README.md",
  "\n    rashomon watch\n",
  "\n    rashomon run -- claude\n    rashomon watch\n",
  "TestSurface_OffersNoDormantProxyPath")
# The installer places one binary. A second fetch is the loop-0 defect: the
# closed proxy's archive downloaded because a release happened to carry it.
m("install.sh fetches a second archive", "install.sh",
  "say \"  installed ${INSTALL_DIR}/rashomon\"\n",
  "say \"  installed ${INSTALL_DIR}/rashomon\"\n"
  "fetch \"${BASE_URL}/${TAG}/altrace_${VERSION}_${OS}_${ARCH}.tar.gz\" \"$TMP/altrace.tar.gz\" || true\n",
  "TestInstall_")
m("install.sh drops the alpha's closing line", "install.sh",
  "say \"Network destinations are not observed in this alpha. Declarations and\"\n",
  "say \"Declarations and\"\n",
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
m("usage advertises the dormant env command", "cmd/rashomon/main.go",
  "  rashomon version               print the version\n",
  "  rashomon env [--port N]        print the proxy variables to export\n"
  "  rashomon version               print the version\n",
  "TestUsage_AdvertisesNoDormantProxyPath")
