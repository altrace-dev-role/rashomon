package core

func init() {
	registry["c"] = Compute([]int{9, 8, 7})
	registry["c2"] = Compute([]int{6, 5, 4})
	registry["c3"] = Compute([]int{3, 2, 1})
}

func _() { registry["y"] = 2 }
