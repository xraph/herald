package push_test

import (
	"bytes"
	"testing"

	"github.com/xraph/herald/driver/push"
)

func TestSignatureBase(t *testing.T) {
	body := []byte(`{"a":1}`)
	got := push.SignatureBase("1793220240", body)
	want := []byte(`1793220240.{"a":1}`)
	if !bytes.Equal(got, want) {
		t.Fatalf("SignatureBase = %q, want %q", got, want)
	}

	for i := range got {
		got[i] = 'x'
	}
	if string(body) != `{"a":1}` {
		t.Fatalf("mutating the result changed the input body: %q", body)
	}
}
