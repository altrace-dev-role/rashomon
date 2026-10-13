package render

import "github.com/altrace-dev-role/rashomon/internal/steer/pattern"

// SampleMessage is a rendered regression-location Message from this
// package's test table, for the tests in package render_test that need a
// non-zero Message, which only New builds.
func SampleMessage() (Message, error) {
	return newMessage(testTable, pattern.RegressionLocation, goodExample())
}
