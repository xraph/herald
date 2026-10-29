// Package pushgateway is an HTTP gateway that registers devices and relays
// sealed pushes to FCM and APNs without ever seeing the notification content.
//
// A client registers a device token and gets back an opaque handle. Only the
// gateway can open a handle, so the sending server never learns the token.
// The package never imports a driver module: callers inject drivers as
// driver.Driver values.
package pushgateway

import (
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"regexp"
	"time"

	"github.com/xraph/herald/driver"
)

const (
	defaultServerPerSecond  = 50
	defaultHandlePerMinute  = 30
	defaultPlaceholderTitle = "Notification"
	defaultPlaceholderBody  = "New notification"
)

var keyIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// SealingKey is an AES-256-GCM key that seals and opens device handles.
type SealingKey struct {
	ID  string // appears in handles as h1.<ID>.<...>; 1 to 32 chars of [A-Za-z0-9_-]
	Key []byte // 32 bytes, AES-256-GCM
}

// Platform is one delivery route: a Herald driver and the provider
// credentials and settings it reads, as Herald providers carry them.
type Platform struct {
	Driver      driver.Driver
	Credentials map[string]string
	Settings    map[string]string
}

// Config configures a gateway.
type Config struct {
	SealingKeys      []SealingKey     // [0] seals new handles; every entry opens them
	Android          *Platform        // FCM; nil: android registrations answer 400
	IOS              *Platform        // APNs; nil: ios registrations answer 400. The gateway sets setting "sandbox" per send from the handle's apns_env
	ServerPerSecond  float64          // per server key; 0 means 50; burst = ceil(rate), at least 1
	HandlePerMinute  float64          // per handle; 0 means 30; burst = ceil(rate), at least 1
	Logger           *slog.Logger     // nil means slog.Default()
	Now              func() time.Time // nil means time.Now; addition for tests
	PlaceholderTitle string           // iOS alert shown until the extension decrypts; "" means "Notification"
	PlaceholderBody  string           // "" means "New notification"
}

type gateway struct {
	keys    []SealingKey
	aeads   map[string]cipher.AEAD
	android *Platform
	ios     *Platform

	serverPerSecond float64
	handlePerMinute float64
	serverLimits    *limiterSet
	handleLimits    *limiterSet

	log              *slog.Logger
	now              func() time.Time
	placeholderTitle string
	placeholderBody  string
}

// New validates cfg and returns a handler serving POST /v1/registrations and
// POST /v1/send.
func New(cfg Config) (http.Handler, error) {
	if len(cfg.SealingKeys) == 0 {
		return nil, errors.New("pushgateway: at least one sealing key is required")
	}
	g := &gateway{
		keys:             append([]SealingKey(nil), cfg.SealingKeys...),
		aeads:            make(map[string]cipher.AEAD, len(cfg.SealingKeys)),
		android:          cfg.Android,
		ios:              cfg.IOS,
		serverPerSecond:  orDefault(cfg.ServerPerSecond, defaultServerPerSecond),
		handlePerMinute:  orDefault(cfg.HandlePerMinute, defaultHandlePerMinute),
		log:              cfg.Logger,
		now:              cfg.Now,
		placeholderTitle: orString(cfg.PlaceholderTitle, defaultPlaceholderTitle),
		placeholderBody:  orString(cfg.PlaceholderBody, defaultPlaceholderBody),
	}
	g.serverLimits = newLimiterSet(g.serverPerSecond, g.serverPerSecond)
	g.handleLimits = newLimiterSet(g.handlePerMinute/60, g.handlePerMinute)
	if g.log == nil {
		g.log = slog.Default()
	}
	if g.now == nil {
		g.now = time.Now
	}
	for i, k := range cfg.SealingKeys {
		if !keyIDPattern.MatchString(k.ID) {
			return nil, fmt.Errorf("pushgateway: sealing key %d has an invalid id", i)
		}
		if _, dup := g.aeads[k.ID]; dup {
			return nil, fmt.Errorf("pushgateway: sealing key id %q is duplicated", k.ID)
		}
		if len(k.Key) != 32 {
			return nil, fmt.Errorf("pushgateway: sealing key %q must be 32 bytes", k.ID)
		}
		block, err := aes.NewCipher(k.Key)
		if err != nil {
			return nil, fmt.Errorf("pushgateway: sealing key %q: %w", k.ID, err)
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, fmt.Errorf("pushgateway: sealing key %q: %w", k.ID, err)
		}
		g.aeads[k.ID] = aead
	}
	if cfg.Android == nil && cfg.IOS == nil {
		return nil, errors.New("pushgateway: at least one platform is required")
	}
	for name, p := range map[string]*Platform{"android": cfg.Android, "ios": cfg.IOS} {
		if p == nil {
			continue
		}
		if p.Driver == nil {
			return nil, fmt.Errorf("pushgateway: %s platform has no driver", name)
		}
		if err := p.Driver.Validate(p.Credentials, p.Settings); err != nil {
			return nil, fmt.Errorf("pushgateway: %s platform: %w", name, err)
		}
	}
	return g, nil
}

func orDefault(v, def float64) float64 {
	if v <= 0 || math.IsNaN(v) {
		return def
	}
	return v
}

func orString(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// platform returns the configured route for a platform name, or nil.
func (g *gateway) platform(name string) *Platform {
	switch name {
	case platformAndroid:
		return g.android
	case platformIOS:
		return g.ios
	}
	return nil
}

// ServeHTTP routes the two gateway paths.
func (g *gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var h http.HandlerFunc
	switch r.URL.Path {
	case "/v1/registrations":
		h = g.serveRegistration
	case "/v1/send":
		h = g.serveSend
	default:
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	h(w, r)
}
