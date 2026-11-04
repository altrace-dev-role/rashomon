package main

import (
	"context"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/intent"
	"github.com/altrace-dev-role/rashomon/internal/platform"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// platformRead is StopInput.Platform for the ledger verb's replay (card
// PL-WIRE1) and, through stopPlatformRead, for the Stop hook, whose line
// names the changed packages whose go test has not run (PW1b-ON-230 step
// 2); the session report and the catch-up pass none. It is
// internal/platform's Read of the pgraph log `pgraph sync <root>`, or the
// plugin's hook, wrote under the project root, taken only as the pair the
// walk file vouches for; a project nobody synced reads as no
// log, every family unknown, never as an unreadable one (platform.Read reads
// a log that is not there as none). What it reads, a person running the verb
// reads: the log, its digests, the go.mod files from each written file's
// directory up to the root, every file the digests list for a written
// package, the written Go files, and each written package's directory
// listing (for a test file). It writes nothing: pgraph is a command of its
// own that a person runs, and this binary starts no process (H-17).
func platformRead(root string, written []string) verify.PlatformRead {
	return platformReadWithin(root, written, 0)
}

// stopPlatformGuard is the Stop's guard on its read, well inside the Stop
// entry's 10 s timeout: a read that stalls reads LogTimedOut and the Stop
// still prints its own line. This repository's 3 MB log decodes in 11 to
// 17 ms (internal/platform's deadline note).
const stopPlatformGuard = 2 * time.Second

// stopPlatformRead is platformRead under the Stop's guard.
func stopPlatformRead(root string, written []string) verify.PlatformRead {
	return platformReadWithin(root, written, stopPlatformGuard)
}

// promptPlatformGuard is the prompt's guard on its symbol read, well inside
// the UserPromptSubmit entry's 10 s timeout, as the Stop's is inside its own.
const promptPlatformGuard = 2 * time.Second

// promptSymbols is intent's Project.Symbols for a prompt resolved against
// root (PW1b-ON-230 step 3): platform.Symbols of the pair the walk file
// vouches for under root, under promptPlatformGuard. What it reads, a person
// running pgraph reads: the walk file, the log, the digests, and every file
// the digests list for the packages its answer rests on. It writes nothing.
func promptSymbols(root string) func(names []string) []intent.Symbol {
	return func(names []string) []intent.Symbol {
		ctx, cancel := context.WithTimeout(context.Background(), promptPlatformGuard)
		defer cancel()
		var out []intent.Symbol
		for _, s := range platform.Symbols(ctx, root, names) {
			out = append(out, intent.Symbol{Name: s.Name, Dir: s.Dir, Callers: s.Callers, Packages: s.CallerPackages})
		}
		return out
	}
}

func platformReadWithin(root string, written []string, guard time.Duration) verify.PlatformRead {
	run := platform.RunView{Root: root, WrittenPaths: written, Deadline: guard}
	if root != "" {
		run.LogPath = platform.LogPath(root)
	}
	return platform.Read(run)
}

// platformHeader is the ledger's line on the pgraph log, true to what the
// replay did: what it read where it read the log, why its rows read unknown
// where it could not, and nothing where there was no log to read (each row
// then names its package @unknown) or no Go file was written.
func platformHeader(log verify.LogState) string {
	switch log {
	case verify.LogRead:
		return "platform: read now, not at the Stop: pgraph log, digests, go.mod, Go files\n"
	case verify.LogUnreadable:
		return "platform: the pgraph log could not be read whole, so its row reads unknown\n"
	case verify.LogTimedOut:
		return "platform: the pgraph log was not read in time, so its row reads unknown\n"
	}
	return ""
}
