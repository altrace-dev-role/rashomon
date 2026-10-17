package supervise

import (
	"bytes"
	"os"
	"strconv"
)

// selfMaxRSS is this process's own peak resident set, in bytes. Linux folds
// the peak of the image exec replaced, the forked copy of whatever started
// this process, into getrusage's maxrss, so the figure is read from VmHWM,
// the high-water mark of this image alone, and from getrusage only when
// /proc cannot be read.
func selfMaxRSS() int64 {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return rusageMaxRSS()
	}
	for _, line := range bytes.Split(b, []byte{'\n'}) {
		rest, ok := bytes.CutPrefix(line, []byte("VmHWM:"))
		if !ok {
			continue
		}
		kb, err := strconv.ParseInt(string(bytes.TrimSpace(bytes.TrimSuffix(bytes.TrimSpace(rest), []byte("kB")))), 10, 64)
		if err != nil {
			break
		}
		return kb << 10
	}
	return rusageMaxRSS()
}
