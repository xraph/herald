package driver

import (
	"reflect"
	"testing"
)

func TestDataPayloadForwardsOnlyDataSettings(t *testing.T) {
	got := DataPayload(map[string]string{
		"access_token":   "tok",
		"server_key":     "sk",
		"from":           "no-reply@example.com",
		"data.order_id":  "42",
		"data.":          "no name, dropped",
		"data.deep_link": "app://orders/42",
	})
	want := map[string]string{"order_id": "42", "deep_link": "app://orders/42"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DataPayload = %v, want %v", got, want)
	}
	if DataPayload(map[string]string{"api_key": "x"}) != nil {
		t.Error("no data settings must give nil, so the payload omits data entirely")
	}
}
