package spec

import (
	"bytes"
	"cmp"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
)

// Trust bindings (tech spec 2.2.10; PRD PF-SP2; card SPEC-TRUST1): the user's
// word, given at a terminal by `rashomon spec trust`, that one repository
// spec may reach the agent. A binding names the spec's project, its path and
// its digest, so an edit, being a new digest, matches no binding: Trusted's
// "any edit drops it". The file sits under the store root, never in the
// repository, and carries an HMAC under the store's trust key (D173), so a
// file the verb did not write binds nothing. That is tamper evidence, not a
// boundary, as learn's state.sum is: the key is a file an agent running
// with the user's rights can read.
const (
	// TrustFile is the bindings file's name under the store root.
	TrustFile = "spec-trust.json"
	// TrustKeyFile is the trust key's name under the store root: TrustKeyBytes
	// random bytes, written with the first binding.
	TrustKeyFile = "spec-trust.key"
	// TrustKeyBytes is the trust key's length; a key file of any other length
	// is no key.
	TrustKeyBytes = 32
	// MaxTrustBytes caps the file on read and on write, the project index's
	// cap (verify.MaxIndexBytes).
	MaxTrustBytes = 64 << 10
)

// Binding is one trusted repository spec. ProjectRoot is the project the spec
// sits in as baseline.Key names it, SpecPath the spec file discovery reads,
// SpecDigest the digest Parse gave the bytes the user was shown.
type Binding struct {
	ProjectRoot string `json:"project_root"`
	SpecPath    string `json:"spec_path"`
	SpecDigest  string `json:"spec_digest"`
	BoundAtMS   int64  `json:"bound_at_unix_ms"`
}

// Bindings is the file's bindings: at most one per project and path, sorted
// by ProjectRoot then SpecPath.
type Bindings struct {
	Bindings []Binding `json:"bindings"`
}

// signedBindings is the file's one object: the bindings, and the MAC over
// their bytes exactly as the file holds them (trustMAC).
type signedBindings struct {
	Bindings json.RawMessage `json:"bindings"`
	MAC      string          `json:"mac"`
}

// ErrBindingsUnsigned is a bindings file whose MAC does not check under the
// store's trust key, a file with no key to check it under, or one that is
// not the file the verb writes (over its cap, not its shape, or beside a key
// that cannot be read): something other than `rashomon spec trust` wrote it.
// It binds nothing, and a trust replaces it (D173b).
var ErrBindingsUnsigned = errors.New("spec: the trust bindings do not carry this store's MAC")

// ReadBindings reads the bindings file under storeRoot through fsys, and the
// trust key beside it only when there is a file to check. A missing file is
// no binding and no error. The MAC is checked over the bindings' bytes
// before any binding is decoded: a file with no key to check it under, or
// whose MAC does not match, is ErrBindingsUnsigned, and so is a file over
// MaxTrustBytes, one that is not this shape (one JSON value, no field it
// does not name) and one whose key cannot be read (D173b: QA-2's item 4, so
// the verb replaces it rather than stopping on it). Only a file that cannot
// be read is another error. Discovery reads every error as no binding:
// untrusted, the fail-closed direction.
func ReadBindings(fsys FS, storeRoot string) (*Bindings, error) {
	path := filepath.Join(storeRoot, TrustFile)
	data, err := fsys.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Bindings{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) > MaxTrustBytes {
		return nil, fmt.Errorf("%w: %s is over its %d-byte cap", ErrBindingsUnsigned, path, MaxTrustBytes)
	}
	var f signedBindings
	if err := decodeOne(data, &f); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrBindingsUnsigned, path, err)
	}
	key, err := fsys.ReadFile(filepath.Join(storeRoot, TrustKeyFile))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s: the trust key: %v", ErrBindingsUnsigned, path, err)
	}
	if len(key) != TrustKeyBytes || !hmac.Equal([]byte(f.MAC), []byte(trustMAC(key, f.Bindings))) {
		return nil, fmt.Errorf("%w: %s", ErrBindingsUnsigned, path)
	}
	var b Bindings
	if err := decodeOne(f.Bindings, &b.Bindings); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrBindingsUnsigned, path, err)
	}
	return &b, nil
}

// decodeOne decodes data, one JSON value with no field v does not name, into v.
func decodeOne(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("data after its one value")
	}
	return nil
}

// trustMAC is the hex HMAC-SHA256 under key over the bindings' bytes.
func trustMAC(key, bindings []byte) string {
	m := hmac.New(sha256.New, key)
	m.Write(bindings)
	return hex.EncodeToString(m.Sum(nil))
}

// Holds reports whether a binding names all three of the project, the path
// and the digest. A nil Bindings holds nothing.
func (b *Bindings) Holds(projectRoot, specPath, digest string) bool {
	return b != nil && slices.ContainsFunc(b.Bindings, func(x Binding) bool {
		return x.ProjectRoot == projectRoot && x.SpecPath == specPath && x.SpecDigest == digest
	})
}

// Bind adds x, replacing any binding of its project and path.
func (b *Bindings) Bind(x Binding) {
	b.Unbind(x.ProjectRoot, x.SpecPath)
	b.Bindings = append(b.Bindings, x)
	slices.SortFunc(b.Bindings, func(p, q Binding) int {
		return cmp.Or(cmp.Compare(p.ProjectRoot, q.ProjectRoot), cmp.Compare(p.SpecPath, q.SpecPath))
	})
}

// Unbind removes the binding of the project and path and reports whether
// there was one.
func (b *Bindings) Unbind(projectRoot, specPath string) bool {
	n := len(b.Bindings)
	b.Bindings = slices.DeleteFunc(b.Bindings, func(x Binding) bool {
		return x.ProjectRoot == projectRoot && x.SpecPath == specPath
	})
	return len(b.Bindings) < n
}

// EncodeBindings is b as the bindings file's bytes, signed under key (the
// store's trust key, TrustKeyBytes long), which hook's WriteSpecBindings
// writes: this package writes no file (VER-37f). An encoding over
// MaxTrustBytes is refused: evicting a binding, as the project index evicts a
// family, would untrust another project's spec without a word.
func EncodeBindings(b *Bindings, key []byte) ([]byte, error) {
	if len(key) != TrustKeyBytes {
		return nil, fmt.Errorf("spec: the trust key is %d bytes, not %d", len(key), TrustKeyBytes)
	}
	list := b.Bindings
	if list == nil {
		list = []Binding{}
	}
	raw, err := json.Marshal(list)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(signedBindings{Bindings: raw, MAC: trustMAC(key, raw)})
	if err != nil {
		return nil, err
	}
	if data = append(data, '\n'); len(data) > MaxTrustBytes {
		return nil, fmt.Errorf("spec: the trust bindings would be over their %d-byte cap; untrust a spec first", MaxTrustBytes)
	}
	return data, nil
}
