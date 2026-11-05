package shape

// Shape is a thing with an area.
type Shape interface{ Area() int }

// Square is a square.
type Square struct{ N int }

// Area is the square's area. It is never negative.
func (s *Square) Area() int { return s.N * s.N }

// Sum adds the areas.
func Sum(ss []Shape) int {
	t := 0
	for _, s := range ss {
		t += s.Area()
	}
	return t
}

// Max is the largest side.
const Max = 10
