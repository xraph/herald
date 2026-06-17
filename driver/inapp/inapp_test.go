package inapp_test

import (
	"context"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/inapp"
	"github.com/xraph/herald/message"
)

func TestInappSendReturnsDelivered(t *testing.T) {
	d := &inapp.Driver{}
	if d.Name() != "inapp" || d.Channel() != "inapp" {
		t.Errorf("name/channel = %q/%q", d.Name(), d.Channel())
	}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{To: "user-1", Text: "hi"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusDelivered {
		t.Errorf("status = %q, want delivered", res.Status)
	}
}

func TestInappValidate(t *testing.T) {
	d := &inapp.Driver{}
	if err := d.Validate(nil, nil); err != nil {
		t.Errorf("Validate should always pass, got %v", err)
	}
}
