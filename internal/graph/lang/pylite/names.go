package pylite

import (
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// symbols are the ids of defs, the defs of a file in unit whose module is
// mod. local maps each module def's name to its func symbol: the names a
// bare call resolves to. byKey maps every def's key to its symbol, a method
// being a method symbol of its class path, so a method shares no id with a
// module def of its name and is never a bare name's callee.
func symbols(defs []def, unit, mod string) (local, byKey map[string]graph.ID) {
	local, byKey = map[string]graph.ID{}, map[string]graph.ID{}
	for _, d := range defs {
		if d.class != "" {
			byKey[d.key()] = graph.SymID(Lang, unit, "method", mod+"."+d.key())
			continue
		}
		local[d.name] = graph.SymID(Lang, unit, "func", mod+"."+d.name)
		byKey[d.name] = local[d.name]
	}
	return local, byKey
}

// calleeOf is the symbol call c resolves to. A self.name or cls.name call
// in a method is the method name of the caller's own class, and no symbol
// when that class defines none: a base may, and is not followed, and the
// module's def of the name is never taken for it. Any other call resolves
// as callee does, so a bare name in a method reaches a module def and never
// a method (a class body is no enclosing scope of its methods). A caller
// key with a dot is a method's, since a module def's name holds none.
func (r Resolver) calleeOf(c call, local, byKey map[string]graph.ID, imports map[string]importRef) (graph.ID, bool) {
	if dot := strings.LastIndex(c.caller, "."); dot >= 0 {
		if name, ok := selfName(c.name); ok {
			id, found := byKey[c.caller[:dot]+"."+name]
			return id, found
		}
	}
	return r.callee(c.name, local, imports)
}

// selfName is the name a self. or cls. call names when it is one attribute
// deep, as load in self.load; self.store.load names no method of the class.
func selfName(name string) (string, bool) {
	for _, p := range []string{"self.", "cls."} {
		if rest, ok := strings.CutPrefix(name, p); ok && !strings.Contains(rest, ".") {
			return rest, true
		}
	}
	return "", false
}

// capWords is true for a CapWords name, the case PEP 8 gives a class: an
// upper-case first letter, and not all upper case, which names a constant.
func capWords(s string) bool {
	return s != "" && s[0] >= 'A' && s[0] <= 'Z' && strings.ToUpper(s) != s
}
