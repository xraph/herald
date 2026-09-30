package extension

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/xraph/forge"

	"github.com/xraph/herald"
	"github.com/xraph/herald/store/memory"
)

func b64(n int) string { return base64.StdEncoding.EncodeToString(make([]byte, n)) }

func TestNoKeyMeansNoOptions(t *testing.T) {
	opts, err := Config{}.credentialOptions()
	if err != nil || opts != nil {
		t.Errorf("got %v, %v", opts, err)
	}
}

func TestKeyBecomesACipher(t *testing.T) {
	for _, c := range []struct {
		id, want string
	}{{"", "k1"}, {"prod-2", "prod-2"}} {
		opts, err := Config{CredentialsKey: b64(32), CredentialsKeyID: c.id}.credentialOptions()
		if err != nil {
			t.Fatal(err)
		}
		h, err := herald.New(append([]herald.Option{herald.WithStore(memory.New())}, opts...)...)
		if err != nil {
			t.Fatal(err)
		}
		if h.CredentialKeyID() != c.want {
			t.Errorf("key ID = %q, want %q", h.CredentialKeyID(), c.want)
		}
	}
}

func TestBadKeysNameTheSettingNotTheValue(t *testing.T) {
	const secretish = "not-base64-but-maybe-a-real-secret"
	_, err := Config{CredentialsKey: secretish}.credentialOptions()
	if err == nil || !strings.Contains(err.Error(), "credentials_key") || strings.Contains(err.Error(), secretish) {
		t.Errorf("err = %v", err)
	}
	_, err = Config{CredentialsKey: b64(16)}.credentialOptions()
	if err == nil || !strings.Contains(err.Error(), "credentials_key") {
		t.Errorf("short key: %v", err)
	}
	_, err = Config{CredentialsKey: b64(32), PreviousCredentialsKeys: []CredentialKeyConfig{{ID: "k0", Key: b64(8)}}}.credentialOptions()
	if err == nil || !strings.Contains(err.Error(), "previous_credentials_keys[0]") {
		t.Errorf("bad previous key: %v", err)
	}
	_, err = Config{PreviousCredentialsKeys: []CredentialKeyConfig{{ID: "k0", Key: b64(32)}}}.credentialOptions()
	if err == nil {
		t.Error("previous keys with no current key were accepted")
	}
}

// recordingLogger keeps what Info and Warn were asked to log.
type recordingLogger struct {
	forge.Logger
	lines []string
}

func (r *recordingLogger) record(level, msg string, fields []forge.Field) {
	line := level + " " + msg
	for _, f := range fields {
		line += fmt.Sprintf(" %s=%v", f.Key(), f.Value())
	}
	r.lines = append(r.lines, line)
}

func (r *recordingLogger) Info(msg string, fields ...forge.Field) { r.record("INFO", msg, fields) }
func (r *recordingLogger) Warn(msg string, fields ...forge.Field) { r.record("WARN", msg, fields) }

func TestStartupSaysWhetherCredentialsAreEncrypted(t *testing.T) {
	keyed := &recordingLogger{Logger: forge.NewNoopLogger()}
	logCredentialKey(keyed, "prod-2")
	if len(keyed.lines) != 1 || !strings.HasPrefix(keyed.lines[0], "INFO ") || !strings.Contains(keyed.lines[0], "key_id=prod-2") {
		t.Errorf("with a key: %q", keyed.lines)
	}

	plain := &recordingLogger{Logger: forge.NewNoopLogger()}
	logCredentialKey(plain, "")
	if len(plain.lines) != 1 || !strings.HasPrefix(plain.lines[0], "WARN ") ||
		!strings.Contains(plain.lines[0], "plaintext") || !strings.Contains(plain.lines[0], "credentials_key") {
		t.Errorf("without a key: %q", plain.lines)
	}
}
