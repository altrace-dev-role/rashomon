package report

import "io"

// OverviewOption sets one choice of the overview render.
type OverviewOption func(*overviewOptions)

type overviewOptions struct{ color bool }

// WithOverviewColor paints the overview with ANSI colors.
func WithOverviewColor() OverviewOption { return func(o *overviewOptions) { o.color = true } }

// Overview writes the one-session view of rep (card RPT-1). RED stub.
func Overview(w io.Writer, rep *Report, opts ...OverviewOption) error { return nil }
