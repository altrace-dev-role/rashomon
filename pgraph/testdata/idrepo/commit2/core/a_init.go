package core

var registry = map[string]int{}

func init() {
	registry["a"] = Compute([]int{1, 2, 3})
	registry["a2"] = Compute([]int{4, 5, 6})
	registry["a3"] = Compute(nil)
}

func _() { registry["x"] = 1 }
