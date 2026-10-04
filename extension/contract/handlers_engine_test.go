package contract

import (
	"slices"
	"testing"
)

func TestEngineInfo(t *testing.T) {
	e := newEnv(t, withKey())
	e.deps.DefaultAppID = appA
	e.deps.APIProtected = func() bool { return true }

	got, err := engineInfoHandler(e.deps)(bg, engineInfoRequest{}, as(appA))
	if err != nil {
		t.Fatalf("engine.info: %v", err)
	}
	if got.App.ID != appA || got.App.Label != appA {
		t.Errorf("app = %+v", got.App)
	}
	if !got.Encryption.Configured || got.Encryption.KeyID != "k1" || !got.APIProtected {
		t.Errorf("encryption = %+v apiProtected = %v", got.Encryption, got.APIProtected)
	}
	if got.DefaultLocale != "en" || !slices.Contains(got.Channels, "email") {
		t.Errorf("defaultLocale = %q channels = %v", got.DefaultLocale, got.Channels)
	}
	if !slices.Contains(got.TemplateFuncs, "upper") {
		t.Errorf("templateFuncs = %v", got.TemplateFuncs)
	}
	var fake *DriverInfo
	for i := range got.Drivers {
		if got.Drivers[i].Name == "fake" {
			fake = &got.Drivers[i]
		}
	}
	if fake == nil || fake.Channel != "email" || len(fake.Fields) != 3 || !fake.Fields[0].Secret {
		t.Fatalf("fake driver = %+v", fake)
	}
}

func TestEngineInfoLabelsTheDefaultApp(t *testing.T) {
	e := newEnv(t)
	got, err := engineInfoHandler(e.deps)(bg, engineInfoRequest{}, as(""))
	// as("") carries an empty claim, which is refused; use a principal with
	// no claims to reach the "" app.
	if codeOf(err) != "PERMISSION_DENIED" {
		t.Fatalf("an empty claim must be refused, got %+v, %v", got, err)
	}
	got, err = engineInfoHandler(e.deps)(bg, engineInfoRequest{}, noClaims())
	if err != nil || got.App.ID != "" || got.App.Label != "Default app" || got.Encryption.Configured || got.APIProtected {
		t.Fatalf("got %+v, %v", got, err)
	}
}
