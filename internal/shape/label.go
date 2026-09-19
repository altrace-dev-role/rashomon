package shape

import (
	"encoding/json"
	"path"
	"strings"
)

// Labels. A closed set, for the same reason the verb classes are one: the
// vocabulary must never grow to fit the input, because a label that can carry a
// value from the path is a path that reached the record by another name.
//
// A label says what a path LOOKS LIKE. It is not a finding, it does not say the
// file was secret, and it does not say what the call did to it. Nothing here
// renders with an adjective.
const (
	LabelSSHKey           = "ssh-key"
	LabelCredentialShaped = "credential-shaped"
	LabelCloudConfig      = "cloud-config"
	LabelEnvFile          = "env-file"
	LabelCertificate      = "certificate"

	// LabelNone is a path that matched no row: the tool named a file and the
	// table recognised nothing about it.
	LabelNone = "none"
	// LabelUnknown is a path that could not be read at all -- absent,
	// non-string, or empty. It is distinct from LabelNone, which is a real
	// answer about a real path.
	LabelUnknown = "unknown"
)

// Labels is the vocabulary, for the schema enum test to compare against.
//
// It exists for the reason store.Reasons() and report.Reasons() exist: a
// vocabulary that is not exported through a function drifts from the published
// schema silently, and the drift is only visible once a record in the field
// carries a value the schema forbids.
func Labels() []string {
	return []string{
		LabelSSHKey,
		LabelCredentialShaped,
		LabelCloudConfig,
		LabelEnvFile,
		LabelCertificate,
		LabelNone,
		LabelUnknown,
	}
}

// pathFields are the tools whose input names a file, and the ONE field read for
// each. Nothing else in any input is read by this file.
//
// The list is the specification's, exactly: Read, Edit and Write carry
// file_path, NotebookEdit carries notebook_path. MultiEdit also carries a
// file_path and is deliberately absent -- version 1 labels the four tools the
// specification names, and widening the contract is a specification change, not
// an implementation detail.
//
// Bash is absent on purpose and not by omission: extracting a file target from
// a command line is the recognised-file-command problem, and a label from a
// guess would be a guess.
var pathFields = map[string]string{
	"Read":         "file_path",
	"Edit":         "file_path",
	"Write":        "file_path",
	"NotebookEdit": "notebook_path",
}

// labelRule is one row of the table. Matching is on the case-folded basename
// alone; exact, then prefix, then suffix.
type labelRule struct {
	label  string
	exact  []string
	prefix []string
	suffix []string
}

// labelTable is the fixed pattern list, in evaluation order. The FIRST row that
// matches decides, so the order is part of the contract and a new row goes
// where its convention belongs rather than at the end.
//
// Every row cites the convention it matches. A row is added by naming a
// convention; it is never added by widening a pattern until a path matches.
var labelTable = []labelRule{
	// OpenSSH keeps its key pair under a fixed basename per algorithm
	// (ssh-keygen(1): id_rsa, id_dsa, id_ecdsa, id_ed25519, and the _sk
	// variants), with the public half suffixed .pub. Prefixes rather than
	// exact names, so an id_ed25519_work or an id_rsa.pub still reads as what
	// it is. The label does not separate the public half from the private one:
	// the basename convention does not reliably say which, and the label is
	// not a finding.
	{
		label:  LabelSSHKey,
		exact:  []string{"authorized_keys"},
		prefix: []string{"id_rsa", "id_dsa", "id_ecdsa", "id_ed25519"},
	},

	// dotenv, and direnv's .envrc. The prefix covers .env, .env.local,
	// .env.production and the rest of the per-environment convention.
	{
		label:  LabelEnvFile,
		prefix: []string{".env"},
	},

	// Provider credential and config files, by their documented basenames:
	// kubectl's kubeconfig, gcloud's application_default_credentials.json,
	// gsutil's .boto, s3cmd's .s3cfg, and Terraform state and variable files,
	// which hold provider secrets in cleartext.
	//
	// This row precedes credential-shaped so that a provider's own credential
	// file reads as the provider's, not as the generic shape.
	{
		label:  LabelCloudConfig,
		exact:  []string{"kubeconfig", "application_default_credentials.json", ".boto", ".s3cfg"},
		suffix: []string{".tfstate", ".tfvars", ".kubeconfig"},
	},

	// X.509 material, certificates and their private keys together: PEM and
	// DER encodings, PKCS#12 and PKCS#7 bundles, Java keystores, and the .key
	// and .csr halves of the same convention. One row because they are one
	// naming family; the vocabulary has no separate private-key value and
	// inventing one here would widen a closed set.
	{
		label:  LabelCertificate,
		suffix: []string{".pem", ".crt", ".cer", ".der", ".p12", ".pfx", ".p7b", ".jks", ".keystore", ".key", ".csr"},
	},

	// Tokens, API keys, passwords and keychains, by the basenames the tools
	// that read them document: netrc(5) and its Windows spelling, libpq's
	// .pgpass, git-credential-store's .git-credentials, Apache's .htpasswd,
	// the AWS CLI's credentials file, npm's .npmrc, twine's .pypirc, Docker's
	// legacy .dockercfg, GnuPG's secring.gpg, KeePass's .kdbx, and Apple's
	// keychain files. The secrets. prefix covers the secrets.yaml /
	// secrets.json convention that Kubernetes, Rails and Ansible all share.
	//
	// That prefix is "secret." and "secrets.", not "secret": a bare prefix
	// would swallow secret-santa.md and secret_handshake.go, and a row that
	// matches prose is a row that was widened until something matched.
	{
		label: LabelCredentialShaped,
		exact: []string{
			".netrc", "_netrc", ".pgpass", ".git-credentials", ".htpasswd",
			"credentials", ".npmrc", ".pypirc", ".dockercfg", "secring.gpg",
			"secret", "secrets",
		},
		prefix: []string{"secret.", "secrets."},
		suffix: []string{".kdbx", ".keychain", ".keychain-db", ".gpg", ".asc", ".token"},
	},
}

// Label reports what the file a call names looks like.
//
// The empty string means this tool names no file, and is what a Bash, Agent or
// WebFetch call gets: the record's field is null, which is not the same
// statement as LabelUnknown. Every other return is one of Labels().
//
// The path itself never leaves this function. Only the basename is examined,
// only against the fixed table above, and nothing derived from it is returned:
// the result is a constant from the closed vocabulary or nothing at all.
//
// There is no filesystem access. The path is not opened, stat'd or resolved,
// and this package imports nothing that could.
func Label(toolName string, toolInput json.RawMessage) string {
	field, ok := pathFields[toolName]
	if !ok {
		return ""
	}
	p, ok := pathField(toolInput, field)
	if !ok || p == "" {
		return LabelUnknown
	}
	return labelForBase(basename(p))
}

// pathField reports the named string field of a tool input. A non-object
// input, a missing field, or a field that is not a string all read as absent,
// which the caller turns into LabelUnknown: "the tool should have named a file
// and this input does not" is a real outcome and is not LabelNone.
func pathField(raw json.RawMessage, field string) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return "", false
	}
	v, ok := obj[field]
	if !ok {
		return "", false
	}
	var s string
	if json.Unmarshal(v, &s) != nil {
		return "", false
	}
	return s, true
}

// basename reduces a path to the last element, case-folded.
//
// A leading ~ is dropped rather than resolved. Expanding it would mean reading
// the user's home directory, and it would change nothing that is examined here:
// the basename of ~/.ssh/id_rsa is id_rsa either way. The anchor a ~ implies
// matters to a path RULE, which is Part 3's problem and needs a real home
// directory; it does not matter to a basename.
//
// Backslashes are folded to slashes first, so a Windows-style path the client
// passed through does not read as one long basename.
func basename(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	if p == "~" {
		return ""
	}
	p = strings.TrimPrefix(p, "~/")
	return strings.ToLower(path.Base(p))
}

// labelForBase walks the table in order and returns the first row that matches.
// A base that matches no row is LabelNone: the path was read and recognised as
// nothing, which is an answer.
func labelForBase(base string) string {
	if base == "" || base == "." || base == "/" {
		return LabelUnknown
	}
	for _, r := range labelTable {
		for _, e := range r.exact {
			if base == e {
				return r.label
			}
		}
		for _, p := range r.prefix {
			if strings.HasPrefix(base, p) {
				return r.label
			}
		}
		for _, s := range r.suffix {
			if strings.HasSuffix(base, s) {
				return r.label
			}
		}
	}
	return LabelNone
}
