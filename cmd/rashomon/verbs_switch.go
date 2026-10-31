package main

import (
	"errors"
	"os"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
)

// switchScope reads the optional scope of a switch from args: --user (the
// default) or --project, not both. The words that are not a scope come back.
func switchScope(args []string) (project bool, rest []string, err error) {
	var user bool
	for _, a := range args {
		switch a {
		case "--project":
			project = true
		case "--user":
			user = true
		default:
			rest = append(rest, a)
		}
	}
	if project && user {
		return false, nil, errors.New("--user and --project are two scopes: name one")
	}
	return project, rest, nil
}

// projectKey is the working directory's project (baseline.Key), the name a
// per-project switch is filed under and the hook looks it up by.
func projectKey() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return baseline.Key(cwd), nil
}

// safeText is s with each control or non-printing rune made "?", for a value
// that came from a file a hand may have written.
func safeText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '?'
		}
		return r
	}, s)
}
