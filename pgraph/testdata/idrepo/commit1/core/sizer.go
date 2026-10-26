package core

// MaxSize is the largest size any Sizer reports.
const MaxSize = 100

type Sizer interface{ Size() int }

type Small struct{}

func (Small) Size() int { return 1 }

type Large struct{}

func (Large) Size() int { return 100 }

func Measure(s Sizer) int { return s.Size() }
