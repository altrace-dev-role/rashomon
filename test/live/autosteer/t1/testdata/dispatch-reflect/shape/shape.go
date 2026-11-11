package shape

// Shape is the interface most call sites dispatch through.
type Shape interface{ Area() float64 }

type Namer interface{ Name() string }

// Solid embeds Shape, so a call to Area on a Solid is a call on Shape's method.
type Solid interface {
	Shape
	Volume() float64
}

// sealed has an unexported method: only types of this package can satisfy it,
// and a method spelled seal in another package is a different method.
type sealed interface{ seal() string }

type Circle struct{ R float64 }

func (c Circle) Area() float64 { return 3 * c.R * c.R }
func (c Circle) Name() string  { return "circle" }
func (Circle) seal() string    { return "circle" }

// Square satisfies Shape only through its pointer.
type Square struct{ S float64 }

func (s *Square) Area() float64 { return s.S * s.S }

type Base struct{}

func (Base) Name() string { return "base" }

// Tri gets Name by promotion from Base.
type Tri struct{ Base }

func (Tri) Area() float64 { return 1 }

// Ghost satisfies Shape and is never constructed anywhere.
type Ghost struct{}

func (Ghost) Area() float64 { return 0 }

type Cube struct{ E float64 }

func (c Cube) Area() float64   { return 6 * c.E * c.E }
func (c Cube) Volume() float64 { return c.E * c.E * c.E }
func (Cube) seal() string      { return "cube" }

// Bad is the only error type in the module.
type Bad struct{}

func (Bad) Error() string { return "bad" }

func Seal(s sealed) string { return s.seal() }
