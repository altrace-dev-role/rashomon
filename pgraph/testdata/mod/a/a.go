package a

import "fmt"

type Shape interface {
	Area() float64
	Name() string
}

type Namer interface{ Name() string }

type RW interface {
	Shape
	fmt.Stringer
}

type Base struct{ ID int }

func (b Base) Name() string { return fmt.Sprint(b.ID) }

type Square struct {
	Base
	Side float64
}

func (s *Square) Area() float64 { return s.Side * s.Side }

type Circle struct{ R float64 }

func (c Circle) Area() float64 { return 3 * c.R * c.R }
func (c Circle) Name() string  { return "circle" }

// Wrapper shares Square's underlying struct, so it embeds Base too.
type Wrapper Square

type Alias = Circle

// AliasSq is another name for Square, not a declaration of a type that embeds.
type AliasSq = Square

type Box[T any] struct{ V T }

func (b Box[T]) Get() T       { return b.V }
func (b Box[T]) Name() string { return "box" }

func Total(ss ...Shape) float64 {
	var t float64
	for _, s := range ss {
		t += s.Area()
	}
	return t
}

func Describe(n Namer) string { return n.Name() }

func init() { _ = Total() }

var Default Shape = New(1)

func New(r float64) Shape { return Circle{R: r} }

func Gen[T any](v T) T { return v }

func UseGen() int { return Gen[int](1) + Gen(2) }

func UseBox() int { return Box[int]{V: 1}.Get() }

func Closure() func() { return func() { Helper() } }

func Helper() {}

func Deferred() {
	defer Helper()
	go Helper()
}

func Conv() { _ = float64(1); _ = len("x") }

func Dyn(f func()) { f() }
