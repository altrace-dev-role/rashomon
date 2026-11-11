package graph

import "testing"

// route is the collision fixture's one route.
const route = "GET:/internal/sessions/{}"

// collisionFixture is two exposers of one route, services/auth under
// comp:auth and services/admin under comp:admin, and one consumer in
// services/api whose request is built from AUTH_URL. The consumer's node is
// formed with the unknown scope and status unresolved; nothing in the
// fixture says which exposer it reaches.
func collisionFixture(t *testing.T) *Store {
	t.Helper()
	s := NewStore()
	rev := RevCommit("C1")
	for _, comp := range []string{"auth", "admin"} {
		id := ContractID("http", string(CompID(comp)), route)
		s.AssertNode(Node{ID: id, Kind: "contract", Tier: Algorithmic, Conf: 1.0, Rev: rev})
		s.AssertEdge(Edge{Src: SymID("py", "services/"+comp, "func", comp+".api.get_session"), Pred: Exposes, Dst: id,
			Tier: Algorithmic, Conf: 1.0, Rev: rev})
	}
	consumer := ContractID("http", UnknownScope, route)
	s.AssertNode(Node{ID: consumer, Kind: "contract", Tier: Algorithmic, Conf: 1.0, Status: Unresolved, Rev: rev,
		Attrs: map[string]string{ConsumerBaseAttr: "AUTH_URL"}})
	s.AssertEdge(Edge{Src: SymID("py", "services/api", "func", "api.client.fetch_session"), Pred: Consumes, Dst: consumer,
		Tier: Algorithmic, Conf: 1.0, Rev: rev})
	return s
}

// TestIdentity_SameRouteTwoServicesAreDistinctContracts: contracts-v2
// section 3. Two services exposing one route are two nodes, each scoped by
// its component; the consumer-side node is a third, unresolved, and path
// equality alone yields may_resolve_to at 0.3 per candidate, never
// certain and never resolves_to. The 11-04a grammar had one node
// contract:http:GET:/internal/sessions/{} for all three.
func TestIdentity_SameRouteTwoServicesAreDistinctContracts(t *testing.T) {
	s := collisionFixture(t)
	rev := RevCommit("C1")
	auth := ContractID("http", "comp:auth", route)
	admin := ContractID("http", "comp:admin", route)
	if auth == admin {
		t.Fatalf("scope did not distinguish the exposers: %s", auth)
	}
	for _, id := range []ID{auth, admin} {
		if _, ok := s.Node(id, rev); !ok {
			t.Fatalf("missing %s", id)
		}
	}
	consumer := ContractID("http", UnknownScope, route)
	s.ResolveConsumers(nil, rev, "")
	n, ok := s.Node(consumer, rev)
	if !ok || n.Status != Unresolved {
		t.Fatalf("consumer-side node = %+v ok=%v, want status unresolved", n, ok)
	}
	may := s.Out(consumer, rev, MayResolveTo)
	if len(may) != 2 {
		t.Fatalf("may_resolve_to = %d, want 2", len(may))
	}
	for _, e := range may {
		if e.Certain || e.Conf > 0.3 {
			t.Fatalf("may_resolve_to %s certain=%v conf=%v; string equality is not evidence", e.Dst, e.Certain, e.Conf)
		}
	}
	if res := s.Out(consumer, rev, ResolvesTo); len(res) != 0 {
		t.Fatalf("resolves_to without evidence = %d, want 0", len(res))
	}
}

// TestIdentity_ServiceMapBindsTheConsumer: evidence E1, a declared service
// map binding the consumer's base symbol to comp:auth, writes one
// resolves_to at tier declared, conf 1.0, certain, and marks the consumer
// asserted; the admin candidate stays a may_resolve_to.
func TestIdentity_ServiceMapBindsTheConsumer(t *testing.T) {
	s := collisionFixture(t)
	rev := RevCommit("C1")
	consumer := ContractID("http", UnknownScope, route)
	s.ResolveConsumers(ServiceMap{"AUTH_URL": CompID("auth")}, rev, "decl:.rashomon/graph.json#services.auth")
	res := s.Out(consumer, rev, ResolvesTo)
	if len(res) != 1 {
		t.Fatalf("resolves_to = %d, want 1", len(res))
	}
	e := res[0]
	if e.Dst != ContractID("http", "comp:auth", route) || !e.Certain || e.Conf != 1.0 || e.Tier != Declared ||
		len(e.Prov.Evidence) != 1 {
		t.Fatalf("E1 resolves_to = %+v", e)
	}
	n, _ := s.Node(consumer, rev)
	if n.Status != Asserted {
		t.Fatalf("consumer status after E1 = %s, want asserted", n.Status)
	}
	may := s.Out(consumer, rev, MayResolveTo)
	if len(may) != 1 || may[0].Dst != ContractID("http", "comp:admin", route) {
		t.Fatalf("may_resolve_to after E1 = %+v, want admin only", may)
	}
}

// TestIdentity_HTTPCanonicalReducesParameters: the three spellings of a
// path parameter are one route.
func TestIdentity_HTTPCanonicalReducesParameters(t *testing.T) {
	for _, p := range []string{"/internal/sessions/{id}", "/internal/sessions/<int:id>", "/internal/sessions/:id"} {
		if got := HTTPCanonical("get", p); got != route {
			t.Fatalf("HTTPCanonical(%q) = %q, want %q", p, got, route)
		}
	}
}

// TestIdentity_ScopeWithColonsParses: a scope is an id with a colon of its
// own and a canonical holds colons too; the parser reads by structure.
func TestIdentity_ScopeWithColonsParses(t *testing.T) {
	for _, tc := range []struct{ id, ckind, scope, canonical string }{
		{"contract:http:comp:auth:GET:/internal/sessions/{}", "http", "comp:auth", "GET:/internal/sessions/{}"},
		{"contract:http:?:GET:/internal/sessions/{}", "http", "?", "GET:/internal/sessions/{}"},
		{"contract:table:db:main:public.sessions", "table", "db:main", "public.sessions"},
	} {
		k, sc, c, ok := splitContract(ID(tc.id))
		if !ok || k != tc.ckind || sc != tc.scope || c != tc.canonical {
			t.Fatalf("splitContract(%s) = %q %q %q %v", tc.id, k, sc, c, ok)
		}
	}
	if _, _, _, ok := splitContract("sym:py:x:func:y"); ok {
		t.Fatal("a symbol id parsed as a contract")
	}
}
