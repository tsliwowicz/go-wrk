package loader

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const sessionDeadline = 5 * time.Second

func runSession(t *testing.T, cfg *LoadCfg, ch <-chan *RequesterStats) *RequesterStats {
	t.Helper()
	go cfg.RunSingleLoadSession()
	select {
	case s := <-ch:
		return s
	case <-time.After(sessionDeadline):
		t.Fatalf("RunSingleLoadSession did not return within %v", sessionDeadline)
		return nil
	}
}

func TestRunSingleLoadSession_HappyPath(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(ts.Close)

	ch := make(chan *RequesterStats, 1)
	cfg := NewLoadCfg(1, 1, ts.URL, "", "GET", "", nil, ch, 1000, true, false, false, false, "", "", "", false, nil)

	stats := runSession(t, cfg, ch)

	if stats.NumRequests == 0 {
		t.Fatal("NumRequests = 0, want > 0")
	}
	if stats.NumErrs != 0 {
		t.Errorf("NumErrs = %d, want 0; ErrMap=%v", stats.NumErrs, stats.ErrMap)
	}
	if stats.TotRespSize <= 0 {
		t.Errorf("TotRespSize = %d, want > 0", stats.TotRespSize)
	}
	if stats.TotDuration <= 0 {
		t.Errorf("TotDuration = %v, want > 0", stats.TotDuration)
	}
	if stats.Histogram == nil {
		t.Fatal("Histogram = nil")
	}
	if got := stats.Histogram.TotalCount(); got != int64(stats.NumRequests) {
		t.Errorf("Histogram.TotalCount() = %d, NumRequests = %d", got, stats.NumRequests)
	}
	if len(stats.ErrMap) != 0 {
		t.Errorf("ErrMap not empty: %v", stats.ErrMap)
	}
}

func TestRunSingleLoadSession_ServerErrorsAccumulate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(ts.Close)

	ch := make(chan *RequesterStats, 1)
	cfg := NewLoadCfg(1, 1, ts.URL, "", "GET", "", nil, ch, 1000, true, false, false, false, "", "", "", false, nil)

	stats := runSession(t, cfg, ch)

	if stats.NumRequests != 0 {
		t.Errorf("NumRequests = %d, want 0", stats.NumRequests)
	}
	if stats.NumErrs == 0 {
		t.Fatal("NumErrs = 0, want > 0")
	}

	var key string
	for k := range stats.ErrMap {
		key = k
		break
	}
	if !strings.Contains(key, "status code 500") {
		t.Errorf("ErrMap key = %q, want substring %q", key, "status code 500")
	}
	if stats.ErrMap[key] != stats.NumErrs {
		t.Errorf("ErrMap[%q] = %d, NumErrs = %d", key, stats.ErrMap[key], stats.NumErrs)
	}
}

func TestRunSingleLoadSession_Stop(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.Close)

	ch := make(chan *RequesterStats, 1)
	cfg := NewLoadCfg(30, 1, ts.URL, "", "GET", "", nil, ch, 1000, true, false, false, false, "", "", "", false, nil)

	go cfg.RunSingleLoadSession()

	time.Sleep(100 * time.Millisecond)
	cfg.Stop()

	select {
	case stats := <-ch:
		if stats == nil {
			t.Fatal("stats == nil")
		}
		// Don't assert on counts — we just want to know we exited early.
	case <-time.After(2 * time.Second):
		t.Fatal("session did not exit within 2s after Stop()")
	}
}

func TestRunSingleLoadSession_BadURL(t *testing.T) {
	// Grab a free port on loopback and close the listener immediately, so
	// connections to that address are reliably refused for the test window.
	// (Using a hardcoded port like 1 is technically OS-dependent.)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	ch := make(chan *RequesterStats, 1)
	cfg := NewLoadCfg(1, 1, "http://"+addr, "", "GET", "", nil, ch, 200, true, false, false, false, "", "", "", false, nil)

	stats := runSession(t, cfg, ch)

	if stats.NumRequests != 0 {
		t.Errorf("NumRequests = %d, want 0", stats.NumRequests)
	}
	if stats.NumErrs == 0 {
		t.Fatal("NumErrs = 0, want > 0")
	}
	if len(stats.ErrMap) == 0 {
		t.Fatal("ErrMap empty, want at least one entry")
	}
}

func TestRunSingleLoadSession_BodyAndMethod(t *testing.T) {
	gotPOST := make(chan struct{}, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			select {
			case gotPOST <- struct{}{}:
			default:
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.Close)

	ch := make(chan *RequesterStats, 1)
	cfg := NewLoadCfg(1, 1, ts.URL, "hello", "POST", "", nil, ch, 1000, true, false, false, false, "", "", "", false, nil)

	stats := runSession(t, cfg, ch)

	if stats.NumRequests == 0 {
		t.Errorf("NumRequests = 0, want > 0")
	}

	select {
	case <-gotPOST:
	default:
		t.Error("server never observed a POST request")
	}
}
