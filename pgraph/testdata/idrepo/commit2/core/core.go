package core

// Version shifts every offset below it.
const Version = "2"

// Layout comment added in the second commit.

// Compute is edited in the second commit: same name, new body.
func Compute(xs []int) int {
	total := 0
	for _, x := range xs {
		if x%2 == 0 {
			total += 2 * x
		} else {
			total -= x
		}
	}
	return total
}

// Canonicalize was Normalize; renamed in the second commit and nothing else about it changes.
func Canonicalize(xs []int) []int {
	out := make([]int, 0, len(xs))
	seen := map[int]bool{}
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// Placeholder is a different stub from the deleted Noop.
func Placeholder() {}

// Left and Right have one body between them and are both renamed in the
// second commit, so which new name is which old name cannot be told.
func LeftSquares(n int) int {
	s := 0
	for i := 0; i < n; i++ {
		s += i * i
	}
	return s
}

func RightSquares(n int) int {
	s := 0
	for i := 0; i < n; i++ {
		s += i * i
	}
	return s
}

// Pile was Stack; Queue stays, and both have a
// method spelled Reset.
type Pile struct{ items []int }
type Queue struct{ items []int }

func (s *Pile) Reset()  { s.items = nil }
func (q *Queue) Reset() { q.items = nil }

func (s *Pile) Push(v int) { s.items = append(s.items, v) }

func (s *Pile) Pop() (int, bool) {
	if len(s.items) == 0 {
		return 0, false
	}
	n := len(s.items) - 1
	v := s.items[n]
	s.items = s.items[:n]
	return v, true
}

func New() *Queue { return &Queue{} }

// Double is unchanged by both commits; the init that calls it moves files.
func Double(xs []int) []int {
	out := make([]int, len(xs))
	for i, x := range xs {
		out[i] = 2 * x
	}
	return out
}
