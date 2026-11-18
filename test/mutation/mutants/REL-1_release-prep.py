# REL-1: a release publishes the plugin archive under the version the
# plugin's manifest names. The build refuses any other version; the release
# job attaches the archive before the release leaves draft; GoReleaser makes
# a draft with a fixed header and no changelog; a dispatch of CI runs the
# pipeline with nothing published.
m("REL-1 the plugin build takes any version", "scripts/build-plugin.sh",
  'if [ "$version" != dev ] && [ "$version" != "$want" ]; then', "if false; then", "TestREL1_")
m("REL-1 the release is never published", ".github/workflows/release.yaml",
  " --draft=false --latest", " --latest", "TestREL1b_")
m("REL-1 the release is public before the archive", ".goreleaser.yml",
  "release:\n  draft: true\n", "release:\n  draft: false\n", "TestREL1c_")
m("REL-1 the dry run publishes nothing it can check", ".github/workflows/ci.yaml",
  "          args: release --snapshot --clean\n", "          args: check\n", "TestREL1d_")

# The release body is a fixed header until the notes are set by hand: no
# changelog, and one short header with no template action and no footer.
_HEADER = ('  header: "This is an alpha release of rashomon. The notes for this version are added to this page'
           ' once the release build has finished. The README in the repository has the install steps."\n')
m("REL-1 the release body lists every commit", ".goreleaser.yml",
  '  disable: "true"\n', '  disable: "false"\n', "TestREL1e_")
m("REL-1 the release has no header", ".goreleaser.yml",
  '\n  header: "', '\n  # header: "', "TestREL1f_")
m("REL-1 the release header is empty", ".goreleaser.yml",
  _HEADER, '  header: ""\n', "TestREL1f_")
m("REL-1 the release header is long", ".goreleaser.yml",
  _HEADER, '  header: "' + "x" * 400 + '"\n', "TestREL1f_")
m("REL-1 the release header renders a template", ".goreleaser.yml",
  "an alpha release of rashomon.", "an alpha release of {{ .ProjectName }}.", "TestREL1f_")
m("REL-1 the release has a footer", ".goreleaser.yml",
  "  draft: true\n", '  draft: true\n  footer: "x"\n', "TestREL1f_")
