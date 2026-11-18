# REL-1: a release publishes the plugin archive under the version the
# plugin's manifest names. The build refuses any other version; the release
# job attaches the archive before the release leaves draft; GoReleaser makes
# a draft; a dispatch of CI runs the pipeline with nothing published.
m("REL-1 the plugin build takes any version", "scripts/build-plugin.sh",
  'if [ "$version" != dev ] && [ "$version" != "$want" ]; then', "if false; then", "TestREL1_")
m("REL-1 the release is never published", ".github/workflows/release.yaml",
  " --draft=false --latest", " --latest", "TestREL1b_")
m("REL-1 the release is public before the archive", ".goreleaser.yml",
  "release:\n  draft: true\n", "release:\n  draft: false\n", "TestREL1c_")
m("REL-1 the dry run publishes nothing it can check", ".github/workflows/ci.yaml",
  "          args: release --snapshot --clean\n", "          args: check\n", "TestREL1d_")
