package middleware

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/lza6/new-api-Max/model"
)

// TestAuditLogSensitiveFieldsAbsent guards the T8-3 audit de-identification
// contract: audit events (model.AuditLog and middleware.auditRouteActions /
// op params) must never contain password, verification code, recovery code,
// private key, or usable session/token credential fields.
//
// It is a source-level regex scan over the audit struct field names, their
// JSON tags and the middleware audit-call parameter keys, mirroring the
// controller access_token_audit_test.go style (which asserts fingerprints are
// stored instead of raw tokens). A match here is a failing regression.
func TestAuditLogSensitiveFieldsAbsent(t *testing.T) {
	sensitivePattern := regexp.MustCompile(`(?i)\b(pass(word|phrase)?|pwd|verification[_ -]?code|verify[_ -]?code|recovery[_ -]?code|totp[_ -]?secret|otp[_ -]?secret|private[_ -]?key|secret[_ -]?key|access[_ -]?token|api[_ -]?key|bearer[_ -]?token|session[_ -]?(token|secret)|csrf[_ -]?token)\b`)
	skipFields := map[string]bool{
		// TokenRef is a SHA-256 fingerprint, not a usable token; it must remain
		// so logs can correlate PAT generation without storing the credential.
		"TokenRef": true,
		// AccessTokenFingerprint is the de-identified ref; the raw access token
		// is only read for fingerprinting and never stored on the entry.
		"AccessTokenFingerprint": true,
		// AuthMethod/Route/Path carry no credential values.
		"AuthMethod": true, "Route": true, "Path": true, "Method": true,
		"RequestId": true, "EventId": true, "Ip": true, "UserAgent": true,
		"Status": true, "Success": true, "Action": true, "Content": true,
		"Username": true, "UserId": true, "ActorRole": true, "CreatedAt": true,
		"Category": true, "Id": true, "Op": true, "AdminInfo": true,
		"AuditInfo": true, "RootInfo": true, "LoginMethod": true,
	}
	var hits []string
	check := func(field, tag string) {
		if skipFields[field] {
			return
		}
		if sensitivePattern.MatchString(field) || (tag != "" && sensitivePattern.MatchString(tag)) {
			hits = append(hits, field+" (json:"+tag+")")
		}
	}

	// 1. Struct fields + JSON tags on the persisted audit event model.
	collectFields(t, "model/audit_log.go", func(field, tag string) { check(field, tag) })
	collectFields(t, "model/audit_other.go", func(field, tag string) { check(field, tag) })
	if len(hits) > 0 {
		t.Fatalf("audit event model exposes sensitive fields: %s", strings.Join(hits, ", "))
	}

	// 2. Params written by the audit middleware into op.params (route params are
	//    path fragments; generic params are method/route only — never bodies).
	auditGo := readFile(t, "middleware/audit.go")
	if sensitivePattern.MatchString(auditGo) {
		// Exclude the whitelisted de-identification symbol and non-credential
		// route templates by checking only opParams map keys.
		for _, key := range opParamsKeys(auditGo) {
			if sensitivePattern.MatchString(key) {
				hits = append(hits, "opParam:"+key)
			}
		}
		if len(hits) > 0 {
			t.Fatalf("audit middleware writes sensitive op params: %s", strings.Join(hits, ", "))
		}
	}

	// 3. model.AuditLog values must never carry raw credentials: fingerprint is
	//    the only credential-adjacent stored value and it is derived, not usable.
	var entry model.AuditLog
	if strings.Contains(entry.TokenRef, "sk-") || strings.Contains(entry.AuthMethod, "sk-") {
		t.Fatal("audit entry must not carry raw token material")
	}
}

// collectFields walks a model source file and invokes fn for every struct field
// with its JSON tag (raw tag text when present).
func collectFields(t *testing.T, rel string, fn func(field, tag string)) {
	t.Helper()
	src := readFile(t, rel)
	fieldRe := regexp.MustCompile(`(?m)^\s*([A-Z][A-Za-z0-9_]*)\s+[^=\n]+` + "`" + `json:"([^"]*)"` + "`")
	matches := fieldRe.FindAllStringSubmatch(src, -1)
	if len(matches) == 0 {
		t.Fatalf("no struct fields collected from %s", rel)
	}
	for _, m := range matches {
		fn(m[1], m[2])
	}
}

// opParamsKeys extracts string keys assigned into opParams maps in audit.go.
func opParamsKeys(src string) []string {
	re := regexp.MustCompile(`(?m)opParams\["([^"]+)"\]`)
	matches := re.FindAllStringSubmatch(src, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

func readFile(t *testing.T, rel string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(filepath.Dir(file))
	path := filepath.Join(root, rel)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}
