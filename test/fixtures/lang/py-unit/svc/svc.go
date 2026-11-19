// Package svc is the fixture's one Go package: the Go side of a repository
// that also declares a Python unit (LANG-REG1).
package svc

import "strings"

// Name is the service name in upper case.
func Name(s string) string { return strings.ToUpper(s) }
