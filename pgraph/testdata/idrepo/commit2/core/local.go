package core

// Pairs and Triples each declare a local type spelled shower, and Both
// declares two in nested scopes: names that repeat without being the same
// symbol.
func Pairs(v any) string {
	type shower interface{ Show() string }
	if s, ok := v.(shower); ok {
		return s.Show()
	}
	return ""
}

func Triples(v any) string {
	type shower interface{ Show() string }
	if s, ok := v.(shower); ok {
		return s.Show()
	}
	return ""
}

func Both() int {
	type pair struct{ a int }
	n := pair{1}.a
	{
		type pair struct{ a, b int }
		n += pair{1, 2}.b
	}
	return n
}

// CloseIt calls a method of an anonymous interface.
func CloseIt(c interface{ Close() error }) error { return c.Close() }
