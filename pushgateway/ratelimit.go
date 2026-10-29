package pushgateway

import (
	"math"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	limiterIdleAfter    = 10 * time.Minute
	limiterSweepEvery   = time.Minute
	limiterMinRetrySecs = 1
)

type limiterEntry struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

// limiterSet is a map of token buckets keyed by string. Entries idle for ten
// minutes are dropped, and the sweep runs at most once a minute.
type limiterSet struct {
	mu        sync.Mutex
	m         map[string]*limiterEntry
	limit     rate.Limit
	burst     int
	lastSweep time.Time
}

// newLimiterSet returns a set whose buckets refill perSecond tokens a second
// and hold ceil(burstOf) tokens, at least one.
func newLimiterSet(perSecond, burstOf float64) *limiterSet {
	return &limiterSet{
		m:     make(map[string]*limiterEntry),
		limit: rate.Limit(perSecond),
		burst: max(1, int(math.Ceil(burstOf))),
	}
}

// allow takes one token from key's bucket at now. On refusal it takes
// nothing and returns the whole seconds until a token is available, rounded
// up, at least 1.
func (s *limiterSet) allow(key string, now time.Time) (ok bool, retryAfter int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if now.Sub(s.lastSweep) >= limiterSweepEvery {
		for k, e := range s.m {
			if now.Sub(e.lastSeen) >= limiterIdleAfter {
				delete(s.m, k)
			}
		}
		s.lastSweep = now
	}
	e, found := s.m[key]
	if !found {
		e = &limiterEntry{lim: rate.NewLimiter(s.limit, s.burst)}
		s.m[key] = e
	}
	e.lastSeen = now

	r := e.lim.ReserveN(now, 1)
	if !r.OK() {
		return false, limiterMinRetrySecs
	}
	delay := r.DelayFrom(now)
	if delay <= 0 {
		return true, 0
	}
	r.CancelAt(now)
	return false, max(limiterMinRetrySecs, int(math.Ceil(delay.Seconds())))
}
