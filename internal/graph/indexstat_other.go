//go:build !unix

package graph

import "os"

// indexAt reports whether p, an index's path, can be stat'd. Where there is
// no unix stat (Windows), os.Stat is the probe.
func indexAt(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
