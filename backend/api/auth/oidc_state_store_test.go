package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/certvault/certvault/config"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

func TestOIDCAbandonedFlowsExpireAutomatically(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var store oidcStateStore
		t.Cleanup(store.close)

		key, _, ok := store.reserve("peer")
		if !ok {
			t.Fatal("flow not admitted")
		}

		time.Sleep(oidcStateLifetime + time.Second)
		synctest.Wait()
		store.mu.Lock()
		empty := len(store.flows) == 0 && len(store.counts) == 0 && len(store.clients) == 0 && store.timer == nil
		store.mu.Unlock()

		if !empty {
			t.Fatal("abandoned entries or cleanup timer remain")
		}

		if _, exists, _ := store.take(key, "unused"); exists {
			t.Fatal("expired state available")
		}
	})
}

func TestOIDCFlowAdmissionBoundsUnderSustainedTraffic(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var store oidcStateStore
		t.Cleanup(store.close)

		for i := range oidcMaxStates {
			if _, _, ok := store.reserve(fmt.Sprintf("peer-%d", i)); !ok {
				t.Fatalf("flow %d rejected prematurely", i)
			}

			time.Sleep(time.Second / time.Duration(oidcGlobalLoginRate))
		}

		for range 1000 {
			if _, _, ok := store.reserve("extra-peer"); ok {
				t.Fatal("state capacity exceeded")
			}
		}

		store.mu.Lock()
		bounded := len(store.flows) == oidcMaxStates && len(store.clients) <= oidcMaxRateClients && len(store.counts) <= oidcMaxStates
		store.mu.Unlock()

		if !bounded {
			t.Fatal("store unbounded")
		}

		time.Sleep(oidcStateLifetime + oidcStateSweepInterval)
		synctest.Wait()

		if _, _, ok := store.reserve("recovered-peer"); !ok {
			t.Fatal("admission did not recover after expiry")
		}
	})
}

func TestOIDCPerClientRateAndOutstandingLimits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var store oidcStateStore
		t.Cleanup(store.close)

		for range oidcLoginBurst {
			key, flow, ok := store.reserve("peer")
			if !ok {
				t.Fatal("initial burst rejected")
			}

			store.take(key, flow.browserToken)
		}

		if _, _, ok := store.reserve("peer"); ok {
			t.Fatal("callbacks bypassed per-client rate limit")
		}

		time.Sleep(time.Minute)
		synctest.Wait()

		for range oidcMaxStatesPerIP {
			if _, _, ok := store.reserve("peer"); !ok {
				t.Fatal("allowed pending flow rejected")
			}

			time.Sleep(time.Minute / oidcLoginBurst)
		}

		if _, _, ok := store.reserve("peer"); ok {
			t.Fatal("per-client outstanding limit exceeded")
		}

		store.mu.Lock()
		count := len(store.flows)
		store.mu.Unlock()

		if count != oidcMaxStatesPerIP {
			t.Fatal("per-client state store grew")
		}
	})
}

func TestOIDCConcurrentAdmissionAndOneUse(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var store oidcStateStore
		t.Cleanup(store.close)

		var (
			admitted atomic.Int64
			wg       sync.WaitGroup
		)
		for i := range 200 {
			wg.Go(func() {
				if _, _, ok := store.reserve(fmt.Sprintf("peer-%d", i)); ok {
					admitted.Add(1)
				}
			})
		}

		wg.Wait()

		if admitted.Load() > oidcGlobalLoginBurst {
			t.Fatalf("global burst exceeded: %d", admitted.Load())
		}

		store.mu.Lock()

		var key, browserToken string
		for value := range store.flows {
			key = value
			browserToken = store.flows[value].state.browserToken

			break
		}
		store.mu.Unlock()

		var consumed atomic.Int64

		for range 20 {
			wg.Go(func() {
				if _, ok, expired := store.take(key, browserToken); ok && !expired {
					consumed.Add(1)
				}
			})
		}

		wg.Wait()

		if consumed.Load() != 1 {
			t.Fatalf("state consumed %d times", consumed.Load())
		}
	})
}

func TestOIDCLoginAdmissionAndCallbackExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := NewBrowserAuthenticator(&config.Config{Auth: config.Auth{OIDC: &config.OIDC{}}}, nil, nil)
		t.Cleanup(a.states.close)
		a.oidc = &oidc.Provider{}
		a.oauth = &oauth2.Config{ClientID: "client", Endpoint: oauth2.Endpoint{AuthURL: "https://id.example.com/authorize"}}

		var (
			state      string
			flowCookie *http.Cookie
		)

		for i := range oidcLoginBurst + 1 {
			request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/auth/login", nil)
			request.Header.Set("X-Forwarded-For", fmt.Sprintf("192.0.2.%d", i))

			response := httptest.NewRecorder()
			a.Login(response, request)

			if i < oidcLoginBurst {
				if response.Code != http.StatusFound {
					t.Fatalf("login: %d", response.Code)
				}

				location, err := url.Parse(response.Header().Get("Location"))
				if err != nil {
					t.Fatal(err)
				}

				state = location.Query().Get("state")
				flowCookie = response.Result().Cookies()[0]

				if state == "" || location.Query().Get("nonce") == "" || location.Query().Get("code_challenge") == "" {
					t.Fatal("missing state, nonce, or PKCE")
				}
			} else if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") == "" {
				t.Fatal("spoofed forwarding header bypassed admission")
			}
		}
		// Expiry is also enforced during consumption, before the next timer sweep.
		time.Sleep(oidcStateLifetime)

		response := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/auth/callback?state="+state, nil)
		request.AddCookie(flowCookie)
		a.Callback(response, request)

		if response.Code != http.StatusBadRequest {
			t.Fatal("expired callback accepted")
		}

		replay := httptest.NewRecorder()
		a.Callback(replay, request)

		if replay.Code != http.StatusBadRequest {
			t.Fatal("state replay accepted")
		}
	})
}
