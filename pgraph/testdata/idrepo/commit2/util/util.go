package util

// Checksum moved here from package core, unchanged.
func Checksum(b []byte) uint32 {
	var h uint32 = 2166136261
	for _, c := range b {
		h ^= uint32(c)
		h *= 16777619
	}
	return h
}

// New shares its name with core.New and is another symbol.
func New() []byte { return nil }
