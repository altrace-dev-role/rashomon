package render

import "errors"

// AsidePrefix opens a note's aside where it prints, after the note's mirror
// in the one systemMessage, or alone when the agent was sent nothing.
const AsidePrefix = "※ rashomon, not sent to the agent: "

// ErrNothing is returned by New for a sparse message every line of which,
// model's and person's, was left out.
var ErrNothing = errors.New("render: the message has no line to print")

// Aside is the person's lines of the message, "" for a template with none.
func (m Message) Aside() string { return m.aside }
