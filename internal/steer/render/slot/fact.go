package slot

import (
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"unicode"
)

// FactKind is a fact value's kind: the fact-kind allowlist of archive
// 40.3.8 ("AWS profile, AWS region, kube context, kube namespace, package
// index hostname, env var name"), spelled as a spec's `facts[].kind` spells
// them (archive 20.2.8). `sign_in_step`, the one other fact kind, is not a
// FactValue kind.
type FactKind string

const (
	FactAWSProfile       FactKind = "aws_profile"
	FactAWSRegion        FactKind = "aws_region"
	FactKubeContext      FactKind = "kube_context"
	FactKubeNamespace    FactKind = "kube_namespace"
	FactPackageIndexHost FactKind = "package_index_host"
	FactEnvVarName       FactKind = "env_var_name"
)

// AllFactKinds returns the six allowlisted kinds in archive 40.3.8's order.
func AllFactKinds() []FactKind {
	return []FactKind{
		FactAWSProfile, FactAWSRegion, FactKubeContext, FactKubeNamespace,
		FactPackageIndexHost, FactEnvVarName,
	}
}

// FactValue is a fact's value with its kind (archive 40.3.8; tech 4.2.7):
// the FactValue charset, a kind on the allowlist, the secret deny list, the
// entropy test (a bare 40- or 64-hex value fails: no profile, region,
// context, namespace, hostname or name is a bare digest) and hostnames
// only, never a URL, its path or its query. A package index host must be a
// bare hostname and an env var name a bare name.
type FactValue struct {
	Kind  FactKind
	Value string
}

// urlRE finds a URL: a scheme before "://", or a scheme and ":/" at the
// start. An ARN-shaped kube context ("arn:aws:eks:...") is not one.
var urlRE = &lazyRegexp{pattern: `://|^[A-Za-z][A-Za-z0-9+.-]*:/`}

// hostnameRE is a bare DNS hostname: no scheme, port, path or query.
var hostnameRE = &lazyRegexp{pattern: `^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)*[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`}

// hostLabels is the closed list of last labels RefuseHosts reads a dotted
// word against (L13, as ruled). A host under a label off it passes: the
// stated limit, with no public-suffix list in the tree (STR-53 stays
// LEARN-MASK1's).
var hostLabels = []string{"com", "net", "org", "io", "dev", "ai", "app", "cloud", "co", "internal", "local", "corp", "lan"}

// hostSeps are the characters, beside white space, a word is split on
// before RefuseHosts reads it: a host rides inside a URL-less path, a port,
// a key=value, a list or an address ("build.example.com/api",
// "db.corp:8443", "host=db.corp", "db.corp,db2.corp", "ops@db.corp"), and a
// quote or parenthesis that wraps it.
const hostSeps = "/:=,;@()'+"

// RefuseHosts refuses text that names a host (LEARN-DELIVER1, D171 LD2):
// the text is split into words on white space and on hostSeps, and a word,
// with trailing ".,;:)" trimmed, is refused when it is a bare hostname
// (hostnameRE) that holds a dot and ends in a label on hostLabels, so
// "build.example.com", "db.corp:8443" and "host=db.corp" are refused and
// "go.mod", "main.go", "a.b" and "v1.2.3" pass. An IP address is refused as
// well: an IPv4 literal ("10.0.0.5", "10.0.0.5:80"), and an IPv6 one,
// bracketed or not, read as a word split on everything but ":" (the
// approved-text charset has no brackets, so a bracketed one fails there
// first). Approved text may hold a project-relative path a person approved,
// never a host. Unlike a slot check, its error names the word: it is shown
// to the person who wrote the text, at approval.
func RefuseHosts(text string) error {
	for _, w := range strings.FieldsFunc(text, func(r rune) bool { return isHostSep(r, true) }) {
		if isIPLiteral(w) {
			return fmt.Errorf("slot: approved text names the address %q", w)
		}
	}
	for _, w := range strings.FieldsFunc(text, func(r rune) bool { return isHostSep(r, false) }) {
		w = strings.TrimRight(w, ".,;:)")
		if isIPLiteral(w) {
			return fmt.Errorf("slot: approved text names the address %q", w)
		}
		dot := strings.LastIndexByte(w, '.')
		if dot < 0 || !hostnameRE.get().MatchString(w) {
			continue
		}
		if slices.Contains(hostLabels, strings.ToLower(w[dot+1:])) {
			return fmt.Errorf("slot: approved text names the host %q", w)
		}
	}
	return nil
}

// isHostSep reports a character RefuseHosts splits a word on; keepColon
// leaves ":" in the word, for an IPv6 literal.
func isHostSep(r rune, keepColon bool) bool {
	return unicode.IsSpace(r) || strings.ContainsRune(hostSeps, r) && !(keepColon && r == ':')
}

// isIPLiteral reports whether w, with a trailing ".,;:" and a bracket pair
// trimmed, is an IPv4 or IPv6 address.
func isIPLiteral(w string) bool {
	w = strings.TrimRight(w, ".,;:")
	w = strings.TrimSuffix(strings.TrimPrefix(w, "["), "]")
	_, err := netip.ParseAddr(w)
	return err == nil
}

// envNameRE is an environment variable name.
var envNameRE = &lazyRegexp{pattern: `^[A-Za-z_][A-Za-z0-9_]*$`}

func (f FactValue) Validate() error {
	if !slices.Contains(AllFactKinds(), f.Kind) {
		return errors.New("slot: fact kind is not on the allowlist")
	}
	if err := charset("fact value", f.Value, factRE); err != nil {
		return err
	}
	if err := denied("fact value", f.Value); err != nil {
		return err
	}
	if highEntropy(f.Value) {
		return fmt.Errorf("%w: fact value fails the entropy test", ErrSecretShaped)
	}
	if urlRE.get().MatchString(f.Value) {
		return errors.New("slot: fact value is a URL; hostnames only")
	}
	switch f.Kind {
	case FactPackageIndexHost:
		return charset("package index host", f.Value, hostnameRE)
	case FactEnvVarName:
		return charset("env var name", f.Value, envNameRE)
	}
	return nil
}

func (f FactValue) Text() string         { return f.Value }
func (f FactValue) bind() (Value, error) { return checked(f, f.Validate()) }
