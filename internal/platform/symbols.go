package platform

import "context"

// Symbol is what the log proves of one name a request named (PW1b-ON-230
// step 3, WED-SLICE row 1(c)): the symbol's spelling after its package (F,
// T, or T.M for a method), the directory of the package that declares it,
// relative to the root, and its direct callers, the distinct PROVEN calls
// records whose object it is, with the packages they are in.
type Symbol struct {
	Name           string
	Dir            string
	Callers        int
	CallerPackages int
}

// Symbols is what the log at root proves of names. Red: nothing yet.
func Symbols(ctx context.Context, root string, names []string) []Symbol { return nil }

// packageIn is the package of a pgraph id. Red: none yet.
func packageIn(dg *digests, id string) string { return "" }

// keepNamed is decode's keep for names. Red: keeps nothing yet.
func keepNamed(names []string) func(subject, object []byte) bool {
	return func(_, _ []byte) bool { return false }
}
