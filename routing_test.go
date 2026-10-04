package herald

import (
	"errors"
	"strings"
	"testing"

	"github.com/xraph/herald/scope"
	"github.com/xraph/herald/store/memory"
)

func TestCheckRouting(t *testing.T) {
	h := newHerald(t, memory.New(), WithDriver(&recordingDriver{name: "rec", channel: "email"}))
	mine := seedProvider(t, h, "app_a", "mine", "rec", 0, true)
	theirs := seedProvider(t, h, "app_b", "theirs", "rec", 0, true)

	if err := h.CheckRouting(bg, &scope.Config{AppID: "app_a", EmailProviderID: mine.ID.String()}); err != nil {
		t.Errorf("own email provider on the email slot: %v", err)
	}
	if err := h.CheckRouting(bg, &scope.Config{AppID: "app_a"}); err != nil {
		t.Errorf("empty rule: %v", err)
	}
	cases := map[string]*scope.Config{
		"another app's provider": {AppID: "app_a", EmailProviderID: theirs.ID.String()},
		"wrong channel":          {AppID: "app_a", SMSProviderID: mine.ID.String()},
		"malformed id":           {AppID: "app_a", EmailProviderID: "nope"},
	}
	var msgs []string
	for name, cfg := range cases {
		err := h.CheckRouting(bg, cfg)
		if !errors.Is(err, ErrInvalidProvider) {
			t.Errorf("%s: %v, want ErrInvalidProvider", name, err)
			continue
		}
		msgs = append(msgs, err.Error())
	}
	// The foreign ID and the malformed one read alike apart from the ID
	// itself, so the error never confirms an ID exists in another app.
	for _, m := range msgs {
		if !strings.Contains(m, "is not a") || !strings.Contains(m, "provider of this app") {
			t.Errorf("message %q", m)
		}
	}
}
