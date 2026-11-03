package slot

// MaxSaidBytes is the most of the person's words one quote holds.
const MaxSaidBytes = 240

// Said is one of the person's standing directives, quoted verbatim by the
// continuity briefing (COMPACT-SLOT1). Red: no check, no text.
type Said struct {
	words, restates string
	digest, rDigest string
	n, rn           int
}

func NewSaid(words, restates string) Said { return Said{words: words, restates: restates} }

func NewSaidDigest(digest string, n int, restatesDigest string, restatesN int) Said {
	return Said{digest: digest, n: n, rDigest: restatesDigest, rn: restatesN}
}

func (s Said) Keyed() bool            { return s.digest != "" }
func (s Said) Digest() string         { return s.digest }
func (s Said) Bytes() int             { return s.n }
func (s Said) RestatesDigest() string { return s.rDigest }
func (s Said) RestatesBytes() int     { return s.rn }
func (s Said) Validate() error        { return nil }
func (s Said) Text() string           { return "" }
func (s Said) bind() (Value, error)   { return checked(s, s.Validate()) }
