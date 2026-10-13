package slot

import (
	"errors"
	"fmt"
	"slices"
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
