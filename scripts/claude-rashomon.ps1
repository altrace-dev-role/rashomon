# claude-rashomon.ps1 -- start a Claude Code session with the rashomon
# recorder on. Windows twin of scripts/claude-rashomon: checks that claude
# exists, runs `rashomon watch` (idempotent), then starts claude with every
# argument passed through.
#
# Install: go install ./cmd/rashomon   (lands rashomon.exe in "$(go env GOPATH)\bin")
# Run:     powershell -File scripts\claude-rashomon.ps1   (or put scripts\ on PATH)

$ErrorActionPreference = 'Stop'

# claude is checked before watch: watch mutates the user settings file, and a
# launcher that installs hooks and then fails to launch has changed the
# machine as a side effect of a command that visibly did nothing.
if (-not (Get-Command claude -ErrorAction SilentlyContinue)) {
    [Console]::Error.WriteLine("claude-rashomon: no 'claude' on PATH; not installing hooks")
    exit 127
}

# The pinned go/bin location wins over PATH, matching the POSIX launcher.
$rashomon = $null
$goBin = Join-Path $env:USERPROFILE 'go\bin\rashomon.exe'
if (Test-Path -LiteralPath $goBin) {
    $rashomon = $goBin
} else {
    $cmd = Get-Command rashomon -ErrorAction SilentlyContinue
    if ($cmd) { $rashomon = $cmd.Source }
}

if (-not $rashomon) {
    [Console]::Error.WriteLine('claude-rashomon: no rashomon.exe in "$env:USERPROFILE\go\bin" or on PATH')
    [Console]::Error.WriteLine('claude-rashomon: from the rashomon repo:  go install ./cmd/rashomon')
    exit 1
}

# watch output belongs on stderr so `claude -p ... | tool` keeps a clean stdout.
& $rashomon watch 2>&1 | ForEach-Object { [Console]::Error.WriteLine("$_") }
if ($LASTEXITCODE -ne 0) {
    [Console]::Error.WriteLine("claude-rashomon: 'rashomon watch' did not install (refused or failed); starting claude UNRECORDED")
    Start-Sleep -Seconds 2
}

& claude @args
exit $LASTEXITCODE
