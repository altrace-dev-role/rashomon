package b

import "example.com/snap/a"

// G calls F.
func G() int { return a.F() }
