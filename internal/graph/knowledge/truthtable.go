package knowledge

import (
	"bytes"
	"encoding/json"
)

// A truth table (test/prd/truth_table.json) is the repository's audit of
// its own product requirements: rows keyed by the requirement's id with a
// title, a status, the implementation as written, what is missing, and
// the evidence files and symbols. Its rows import as requirements
// (W-M1.12), declared and approved, since the table is the owner's own
// statement of the product.
type truthTable struct {
	Rows []truthRow `json:"rows"`
}

type truthRow struct {
	ID             string          `json:"id"`
	Title          string          `json:"title"`
	Status         string          `json:"status"`
	Implementation string          `json:"implementation"`
	Missing        []string        `json:"missing"`
	Evidence       []truthEvidence `json:"evidence"`
}

type truthEvidence struct {
	File   string `json:"file"`
	Symbol string `json:"symbol"`
}

// ParseTruthTable is the rows of a truth table as requirements: the row's
// id, its title, a clause of the title with its status and the first
// missing item, the implementation as the text, the file and row as the
// source, and each evidence file or file#symbol in Evidence for the
// resolver. ok is false when text is not a truth table: not a JSON object
// with a rows list whose every row carries an id and a title.
func ParseTruthTable(file string, text []byte) ([]Row, bool) {
	if !bytes.HasPrefix(bytes.TrimSpace(text), []byte("{")) {
		return nil, false
	}
	var tt truthTable
	if err := json.Unmarshal(text, &tt); err != nil || len(tt.Rows) == 0 {
		return nil, false
	}
	rows := make([]Row, 0, len(tt.Rows))
	for _, r := range tt.Rows {
		if r.ID == "" || r.Title == "" {
			return nil, false
		}
		clause := r.Title
		if r.Status != "" {
			clause += "; status " + r.Status
		}
		if len(r.Missing) > 0 {
			clause += "; missing: " + r.Missing[0]
		}
		row := Row{ID: r.ID, Kind: KindRequirement, Title: r.Title, Clause: clause, Text: r.Implementation,
			Source: Source{File: file, Row: r.ID}, Status: StatusApproved, Tier: TierDeclared}
		for _, e := range r.Evidence {
			if e.File == "" {
				continue
			}
			ref := e.File
			if e.Symbol != "" {
				ref += "#" + e.Symbol
			}
			row.Evidence = append(row.Evidence, ref)
		}
		rows = append(rows, row)
	}
	return rows, true
}
