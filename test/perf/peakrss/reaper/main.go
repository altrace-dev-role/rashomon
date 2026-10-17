// Command reaper runs one child for peakrss and reports its peak RSS on
// descriptor 3. It is built by peakrss.NewReaper and is not installed.
package main

import (
	"os"

	"github.com/altrace-dev-role/rashomon/test/perf/peakrss"
)

func main() {
	os.Exit(peakrss.Reap(os.Args[1:], os.NewFile(3, "peakrss report")))
}
