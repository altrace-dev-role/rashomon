package app

import "example.test/dispatch/shape"

// Holder satisfies shape.Solid: Area is promoted from the embedded interface
// and Volume is its own.
type Holder struct{ shape.Shape }

func (Holder) Volume() float64 { return 0 }

var DefaultArea = shape.Shape(shape.Circle{R: 2}).Area()

func Total(ss []shape.Shape) float64 {
	var t float64
	for _, s := range ss {
		t += s.Area()
	}
	return t
}

func Label(n shape.Namer) string { return n.Name() }

func Closure(s shape.Shape) func() float64 {
	return func() float64 { return s.Area() }
}

func UseHolder(h Holder) float64 { return h.Area() }

func Vol(s shape.Solid) float64 { return s.Area() + s.Volume() }

func Fail(err error) string { return err.Error() }

// RunAll is the module's one entry point: the values it builds decide which
// methods the call sites above can really reach.
func RunAll() float64 {
	ss := []shape.Shape{shape.Circle{R: 1}, &shape.Square{S: 2}, shape.Tri{}}
	t := Total(ss)
	_ = Label(shape.Circle{}) + Label(shape.Tri{})
	t += Closure(shape.Circle{R: 3})()
	t += UseHolder(Holder{shape.Circle{R: 4}})
	t += Vol(shape.Cube{E: 1}) + Vol(Holder{shape.Circle{R: 5}})
	_ = shape.Seal(shape.Circle{})
	_ = Fail(shape.Bad{})
	return t + DefaultArea
}
