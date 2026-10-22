package budget

import (
	"encoding/json"
	"sync"
)

var (
	tableOnce sync.Once
	table     Budgets
	tableErr  error
)

// Table returns the budgets compiled into the binary, decoded from the
// generated JSON on first use. An error here is a build defect (the
// generated text does not parse) and every caller fails closed on it.
func Table() (Budgets, error) {
	tableOnce.Do(func() {
		tableErr = json.Unmarshal([]byte(tableJSON), &table)
	})
	return table, tableErr
}
