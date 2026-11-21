# PGR-1: pgraph, the indexer, ships beside rashomon. Every release archive
# carries it, install.sh places it when the archive does and says how to
# get it when the archive does not, the notices file covers what it links,
# and the release checks each archive's members before it is published.
# Each row below breaks one of those and names the test that must go red.
# PGR-1 is this file's own identity; it claims no PRD row.

# install.sh never copies the archive's pgraph: the chmod of its temporary
# name then fails under set -eu, and the install stops.
m('PGR-1 install.sh copies no pgraph', 'install.sh',
  '  cp "$TMP/pgraph" "$INSTALL_DIR/.pgraph.incoming" || die "could not write to $INSTALL_DIR"\n',
  '', 'TestInstall_PlacesRashomonAndPgraph')

# An archive with no pgraph, as every earlier release has, stops the install
# instead of placing rashomon and printing the note.
m('PGR-1 an archive without pgraph stops the install', 'install.sh',
  '  say "  this release carries no pgraph; a Go repository needs it (Go 1.26 or later):"\n',
  '  die "  this release carries no pgraph; a Go repository needs it (Go 1.26 or later):"\n', 'TestInstall_AnOlderReleaseWithoutPgraph')

# The copy runs whether or not the archive carries pgraph, so an older
# archive fails at cp.
m('PGR-1 install.sh copies pgraph without looking for it', 'install.sh',
  'if [ -f "$TMP/pgraph" ]; then PGRAPH=yes; else PGRAPH=no; fi\n',
  'if true; then PGRAPH=yes; else PGRAPH=no; fi\n', 'TestInstall_AnOlderReleaseWithoutPgraph')

# The note keeps its first line and loses the command that gets pgraph.
m('PGR-1 the note names no way to get pgraph', 'install.sh',
  '  say "    go install github.com/altrace-dev-role/rashomon/pgraph/cmd/pgraph@main"\n',
  '', 'TestInstall_AnOlderReleaseWithoutPgraph')

# The exit trap removes the download folder alone, as it did before pgraph
# joined the archive, so a failed pgraph copy leaves rashomon's temporary
# name in the install directory.
m('PGR-1 the exit trap leaves a staged binary behind', 'install.sh',
  'trap \'rm -rf "$TMP"; rm -f "$INSTALL_DIR/.rashomon.incoming" "$INSTALL_DIR/.pgraph.incoming"\' EXIT INT TERM HUP\n',
  'trap \'rm -rf "$TMP"\' EXIT INT TERM HUP\n', 'TestInstall_AFailedCopyLeavesNothingStaged')

# The archive stops taking the pgraph build, so every archive ships
# rashomon alone.
m('PGR-1 the archives leave pgraph out', '.goreleaser.yml',
  '      - pgraph\n',
  '', 'TestRelease_PgraphShipsInEveryArchive')

# pgraph is built with cgo on, which the build reader refuses.
m('PGR-1 pgraph is built with cgo on', '.goreleaser.yml',
  '    dir: pgraph\n    main: ./cmd/pgraph\n    binary: pgraph\n    env:\n      - CGO_ENABLED=0\n',
  '    dir: pgraph\n    main: ./cmd/pgraph\n    binary: pgraph\n    env:\n      - CGO_ENABLED=1\n', 'TestRelease_PgraphShipsInEveryArchive')

# The notices file loses a module pgraph links.
m('PGR-1 the notices miss a module pgraph links', 'THIRD_PARTY_NOTICES',
  '  golang.org/x/tools v0.52.0\n',
  '', 'TestThirdPartyNotices_CoversEveryLinkedModule')

# The release is published without a check of what the archives carry.
m('PGR-1 the release skips the archive check', '.github/workflows/release.yaml',
  '      - name: Check what the archives carry\n        run: scripts/check-release-archives.sh dist "$RUNNER_TEMP/rashomon-plugin.zip"\n',
  '', 'TestReleaseArchives_TheReleaseIsCheckedBeforeItIsPublished')

# The check stops looking for pgraph, so an archive without it passes.
m('PGR-1 the archive check does not look for pgraph', 'scripts/check-release-archives.sh',
  '\tfor member in "rashomon$exe" "pgraph$exe" LICENSE THIRD_PARTY_NOTICES; do\n',
  '\tfor member in "rashomon$exe" LICENSE THIRD_PARTY_NOTICES; do\n', 'TestReleaseArchives_CheckScriptRefusesAMissingMember')
