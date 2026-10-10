package auth

import (
	"crypto/hmac"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

const (
	oidcMaxStates          = 1024
	oidcMaxStatesPerIP     = 16
	oidcMaxRateClients     = 1024
	oidcStateSweepInterval = time.Minute
	oidcLoginBurst         = 8
	oidcLoginRate          = float64(8) / 60
	oidcGlobalLoginBurst   = 32
	oidcGlobalLoginRate    = float64(8)
)

type oidcFlow struct {
	state oidcState
	peer  string
}

type loginBucket struct {
	tokens float64
	at     time.Time
}

func (b loginBucket) refill(now time.Time, rate float64, burst int) loginBucket {
	if b.at.IsZero() {
		return loginBucket{tokens: float64(burst), at: now}
	}

	b.tokens = min(float64(burst), b.tokens+now.Sub(b.at).Seconds()*rate)
	b.at = now

	return b
}

// Both flow and rate-limit maps are bounded. One timer expires abandoned flows
// and idle rate-limit entries; it stops when no entries remain.
type oidcStateStore struct {
	mu      sync.Mutex
	flows   map[string]oidcFlow
	counts  map[string]int
	clients map[string]loginBucket
	global  loginBucket
	timer   *time.Timer
	closed  bool
}

func (s *oidcStateStore) reserve(peer string) (string, oidcState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed || len(s.flows) >= oidcMaxStates || s.counts[peer] >= oidcMaxStatesPerIP {
		return "", oidcState{}, false
	}

	now := time.Now()

	global := s.global.refill(now, oidcGlobalLoginRate, oidcGlobalLoginBurst)
	if global.tokens < 1 {
		return "", oidcState{}, false
	}

	client, exists := s.clients[peer]
	if !exists && len(s.clients) >= oidcMaxRateClients {
		return "", oidcState{}, false
	}

	client = client.refill(now, oidcLoginRate, oidcLoginBurst)
	if client.tokens < 1 {
		return "", oidcState{}, false
	}

	key := randomToken()
	state := oidcState{browserToken: randomToken(), nonce: randomToken(), verifier: oauth2.GenerateVerifier(), at: now}

	if s.flows == nil {
		s.flows = make(map[string]oidcFlow)
		s.counts = make(map[string]int)
		s.clients = make(map[string]loginBucket)
	}

	global.tokens--
	client.tokens--
	s.global = global
	s.clients[peer] = client
	s.flows[key] = oidcFlow{state: state, peer: peer}

	s.counts[peer]++
	if s.timer == nil {
		s.timer = time.AfterFunc(oidcStateSweepInterval, s.sweep)
	}

	return key, state, true
}

func (s *oidcStateStore) remove(key string, flow oidcFlow) {
	delete(s.flows, key)

	s.counts[flow.peer]--
	if s.counts[flow.peer] == 0 {
		delete(s.counts, flow.peer)
	}
}

func (s *oidcStateStore) take(key, browserToken string) (oidcState, bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	flow, exists := s.flows[key]
	if !exists || browserToken == "" || !hmac.Equal([]byte(flow.state.browserToken), []byte(browserToken)) {
		return oidcState{}, false, false
	}

	s.remove(key, flow)

	return flow.state, true, !time.Now().Before(flow.state.at.Add(oidcStateLifetime))
}

func (s *oidcStateStore) discard(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if flow, exists := s.flows[key]; exists {
		s.remove(key, flow)
	}
}

func (s *oidcStateStore) sweep() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.timer = nil
	if s.closed {
		return
	}

	now := time.Now()
	for key, flow := range s.flows {
		if !now.Before(flow.state.at.Add(oidcStateLifetime)) {
			s.remove(key, flow)
		}
	}

	for peer, client := range s.clients {
		if !now.Before(client.at.Add(time.Minute)) {
			delete(s.clients, peer)
		}
	}

	if len(s.flows) > 0 || len(s.clients) > 0 {
		s.timer = time.AfterFunc(oidcStateSweepInterval, s.sweep)
	}
}

func (s *oidcStateStore) close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}

	clear(s.flows)
	clear(s.counts)
	clear(s.clients)
}
