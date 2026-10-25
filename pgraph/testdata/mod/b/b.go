package b

import "example.test/mod/a"

type Tri struct{ a.Base }

func (Tri) Area() float64 { return 0 }

type Local interface{ Area() float64 }

func Run() string { return a.Describe(Tri{}) }
