package push_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/driver/push"
)

func serviceAccountJSON(t *testing.T, tokenURL, projectID string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	out, err := json.Marshal(map[string]string{
		"type":           "service_account",
		"project_id":     projectID,
		"private_key_id": "kid1",
		"private_key":    string(keyPEM),
		"client_email":   "sa@" + projectID + ".iam.gserviceaccount.com",
		"token_uri":      tokenURL,
	})
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return string(out)
}

// tokenServer answers every request with ya29.<hit number> and counts hits.
func tokenServer(t *testing.T, expiresIn int, status int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		if status != http.StatusOK {
			w.WriteHeader(status)
			fmt.Fprint(w, `{"error":"invalid_grant"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":"ya29.%d","token_type":"Bearer","expires_in":%d}`, n, expiresIn)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func saSend(t *testing.T, d *push.FCMDriver, fcm *drivertest.Server, data map[string]string) error {
	t.Helper()
	all := map[string]string{"base_url": fcm.URL}
	for k, v := range data {
		all[k] = v
	}
	_, err := d.Send(context.Background(), &driver.OutboundMessage{To: "device-token", Title: "Hi", Text: "body", Data: all})
	return err
}

func TestFCMServiceAccountMintsAndCachesToken(t *testing.T) {
	tok, hits := tokenServer(t, 3600, http.StatusOK)
	fcm := drivertest.NewServer(t, http.StatusOK, `{"name":"n"}`)
	d := &push.FCMDriver{}
	sa := serviceAccountJSON(t, tok.URL, "proj-1")
	for range 2 {
		if err := saSend(t, d, fcm, map[string]string{"service_account": sa}); err != nil {
			t.Fatalf("Send: %v", err)
		}
		if got := fcm.Captured.Header.Get("Authorization"); got != "Bearer ya29.1" {
			t.Errorf("auth = %q", got)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("token hits = %d, want 1", hits.Load())
	}
	if fcm.Captured.Path != "/v1/projects/proj-1/messages:send" {
		t.Errorf("path = %q", fcm.Captured.Path)
	}
}

func TestFCMServiceAccountRefreshesNearExpiry(t *testing.T) {
	tok, hits := tokenServer(t, 200, http.StatusOK)
	fcm := drivertest.NewServer(t, http.StatusOK, `{"name":"n"}`)
	d := &push.FCMDriver{}
	sa := serviceAccountJSON(t, tok.URL, "proj-1")
	for range 2 {
		if err := saSend(t, d, fcm, map[string]string{"service_account": sa}); err != nil {
			t.Fatalf("Send: %v", err)
		}
	}
	if hits.Load() != 2 {
		t.Errorf("token hits = %d, want 2", hits.Load())
	}
	if got := fcm.Captured.Header.Get("Authorization"); got != "Bearer ya29.2" {
		t.Errorf("auth = %q", got)
	}
}

func TestFCMServiceAccountCacheIsPerKey(t *testing.T) {
	tok, hits := tokenServer(t, 3600, http.StatusOK)
	fcm := drivertest.NewServer(t, http.StatusOK, `{"name":"n"}`)
	d := &push.FCMDriver{}
	bearers := make([]string, 0, 2)
	for range 2 {
		sa := serviceAccountJSON(t, tok.URL, "proj-1")
		if err := saSend(t, d, fcm, map[string]string{"service_account": sa}); err != nil {
			t.Fatalf("Send: %v", err)
		}
		bearers = append(bearers, fcm.Captured.Header.Get("Authorization"))
	}
	if hits.Load() != 2 {
		t.Errorf("token hits = %d, want 2", hits.Load())
	}
	if bearers[0] == bearers[1] {
		t.Errorf("both keys got %q", bearers[0])
	}
}

func TestFCMProjectSettingWins(t *testing.T) {
	tok, _ := tokenServer(t, 3600, http.StatusOK)
	fcm := drivertest.NewServer(t, http.StatusOK, `{"name":"n"}`)
	d := &push.FCMDriver{}
	sa := serviceAccountJSON(t, tok.URL, "proj-1")
	if err := saSend(t, d, fcm, map[string]string{"service_account": sa, "project_id": "other"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if fcm.Captured.Path != "/v1/projects/other/messages:send" {
		t.Errorf("path = %q", fcm.Captured.Path)
	}
}

func TestFCMServiceAccountBeatsAccessToken(t *testing.T) {
	tok, _ := tokenServer(t, 3600, http.StatusOK)
	fcm := drivertest.NewServer(t, http.StatusOK, `{"name":"n"}`)
	d := &push.FCMDriver{}
	sa := serviceAccountJSON(t, tok.URL, "proj-1")
	if err := saSend(t, d, fcm, map[string]string{"service_account": sa, "access_token": "static"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := fcm.Captured.Header.Get("Authorization"); got != "Bearer ya29.1" {
		t.Errorf("auth = %q", got)
	}
}

func TestFCMTokenErrorNeverQuotesKey(t *testing.T) {
	tok, _ := tokenServer(t, 3600, http.StatusBadRequest)
	fcm := drivertest.NewServer(t, http.StatusOK, `{"name":"n"}`)
	d := &push.FCMDriver{}
	sa := serviceAccountJSON(t, tok.URL, "proj-1")
	err := saSend(t, d, fcm, map[string]string{"service_account": sa})
	if err == nil {
		t.Fatal("expected error")
	}
	for _, leak := range []string{"PRIVATE KEY", "sa@proj-1.iam.gserviceaccount.com", "invalid_grant"} {
		if strings.Contains(err.Error(), leak) {
			t.Errorf("error leaks %q: %v", leak, err)
		}
	}
}
