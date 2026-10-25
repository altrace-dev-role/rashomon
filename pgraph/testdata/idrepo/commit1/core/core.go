package core

// Compute is edited in the second commit: same name, new body.
func Compute(xs []int) int {
	total := 0
	for _, x := range xs {
		if x%2 == 0 {
			total += x
		} else {
			total -= x
		}
	}
	return total
}

// Normalize is renamed in the second commit and nothing else about it changes.
func Normalize(xs []int) []int {
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

// Checksum moves to package util in the second commit.
func Checksum(b []byte) uint32 {
	var h uint32 = 2166136261
	for _, c := range b {
		h ^= uint32(c)
		h *= 16777619
	}
	return h
}

// Noop is a stub; the second commit deletes it and adds a different stub.
func Noop() {}

// Left and Right have one body between them and are both renamed in the
// second commit, so which new name is which old name cannot be told.
func Left(n int) int {
	s := 0
	for i := 0; i < n; i++ {
		s += i * i
	}
	return s
}

func Right(n int) int {
	s := 0
	for i := 0; i < n; i++ {
		s += i * i
	}
	return s
}

// Stack is renamed Pile in the second commit; Queue stays, and both have a
// method spelled Reset.
type Stack struct{ items []int }
type Queue struct{ items []int }

func (s *Stack) Reset() { s.items = nil }
func (q *Queue) Reset() { q.items = nil }

func (s *Stack) Push(v int) { s.items = append(s.items, v) }

func (s *Stack) Pop() (int, bool) {
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
