# claude-rashomon.ps1 -- start a Claude Code session with the rashomon
# recorder on. Windows twin of scripts/claude-rashomon: checks that claude
# exists, runs `rashomon watch` (idempotent), then starts claude with every
# argument passed through. An invocation that starts no session installs
# nothing: --help, --version and the management subcommands pass straight
# through without watch.
#
# Install: go install ./cmd/rashomon   (lands rashomon.exe in "$(go env GOPATH)\bin")
# Run:     powershell -File scripts\claude-rashomon.ps1   (or put scripts\ on PATH)

$ErrorActionPreference = 'Stop'

# claude is checked before anything else: watch mutates the user settings
# file, and a launcher that installs hooks and then fails to launch has
# changed the machine as a side effect of a command that visibly did nothing.
if (-not (Get-Command claude -ErrorAction SilentlyContinue)) {
    [Console]::Error.WriteLine("claude-rashomon: no 'claude' on PATH; not installing hooks")
    exit 127
}

# The same decision as the POSIX launcher: -h/--help/-v/--version before any
# "--", or a management subcommand named first, mean no session and so no
# watch. Comparisons are case-sensitive (-ceq, -cin); PowerShell's -eq is not.
$watch = $true
foreach ($a in $args) {
    if ($a -ceq '--') { break }
    if ($a -cin @('-h', '--help', '-v', '--version')) { $watch = $false; break }
}
if ($args.Count -gt 0) {
    if ($args[0] -ceq 'help') {
        $watch = $false
    } elseif ($args[0] -cin @('update', 'install', 'doctor', 'auth', 'setup-token', 'mcp', 'plugin', 'auto-mode', 'daemon', 'logs', 'rm', 'stop', 'project')) {
        $watch = $false
        [Console]::Error.WriteLine("claude-rashomon: 'claude $($args[0])' starts no session; watch not run")
    }
}

if ($watch) {
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

    # watch's stdout is moved to stderr so `claude -p ... | tool` keeps a clean
    # stdout. Its own stderr is left alone rather than merged with 2>&1: under
    # Windows PowerShell 5.1 with ErrorActionPreference Stop, a merged native
    # stderr line is a terminating error, and a refusal would kill the
    # launcher instead of reaching the UNRECORDED line below.
    & $rashomon watch | ForEach-Object { [Console]::Error.WriteLine("$_") }
    if ($LASTEXITCODE -ne 0) {
        [Console]::Error.WriteLine("claude-rashomon: 'rashomon watch' did not install (refused or failed); starting claude UNRECORDED")
        Start-Sleep -Seconds 2
    }
}

& claude @args
exit $LASTEXITCODE
