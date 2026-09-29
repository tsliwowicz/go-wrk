package loader

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts
}

func defaultTestClient(t *testing.T) *http.Client {
	t.Helper()
	c, err := client(false, false, false, 5000, true, "", "", "", false)
	if err != nil {
		t.Fatalf("client() err = %v", err)
	}
	return c
}

func TestDoRequest_Success200(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != USER_AGENT {
			t.Errorf("User-Agent = %q, want %q", got, USER_AGENT)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	})

	respSize, dur, err := DoRequest(defaultTestClient(t), nil, "GET", "", ts.URL, "", nil)
	if err != nil {
		t.Fatalf("DoRequest err = %v", err)
	}
	if respSize <= 0 {
		t.Errorf("respSize = %d, want > 0", respSize)
	}
	if dur <= 0 {
		t.Errorf("duration = %v, want > 0", dur)
	}
}

func TestDoRequest_CustomStatusCode(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != USER_AGENT {
			t.Errorf("User-Agent = %q, want %q", got, USER_AGENT)
		}
		w.WriteHeader(http.StatusNotModified)
		_, _ = w.Write([]byte("hello"))
	})

	respSize, dur, err := DoRequest(defaultTestClient(t), nil, "GET", "", ts.URL, "", []int{http.StatusNotModified})
	if err != nil {
		t.Fatalf("DoRequest err = %v", err)
	}
	if respSize <= 0 {
		t.Errorf("respSize = %d, want > 0", respSize)
	}
	if dur <= 0 {
		t.Errorf("duration = %v, want > 0", dur)
	}
}

func TestDoRequest_CustomHeadersAndHost(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "myvhost" {
			t.Errorf("r.Host = %q, want %q", r.Host, "myvhost")
		}
		if got := r.Header.Get("X-Foo"); got != "bar" {
			t.Errorf("X-Foo = %q, want %q", got, "bar")
		}
		w.WriteHeader(http.StatusOK)
	})

	hdr := map[string]string{"X-Foo": "bar"}
	_, _, err := DoRequest(defaultTestClient(t), hdr, "GET", "myvhost", ts.URL, "", nil)
	if err != nil {
		t.Fatalf("DoRequest err = %v", err)
	}
}

func TestDoRequest_Body(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != "payload" {
			t.Errorf("server got body %q, want %q", string(body), "payload")
		}
		if r.Method != "POST" {
			t.Errorf("r.Method = %q, want POST", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	})

	_, _, err := DoRequest(defaultTestClient(t), nil, "POST", "", ts.URL, "payload", nil)
	if err != nil {
		t.Fatalf("DoRequest err = %v", err)
	}
}

func TestDoRequest_QueryEscaping(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("q"); got != "hello world" {
			t.Errorf("q = %q, want %q", got, "hello world")
		}
		w.WriteHeader(http.StatusOK)
	})

	_, _, err := DoRequest(defaultTestClient(t), nil, "GET", "", ts.URL+"/?q=hello world", "", nil)
	if err != nil {
		t.Fatalf("DoRequest err = %v", err)
	}
}

func TestDoRequest_301RedirectBlocked(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://example.invalid/")
		w.WriteHeader(http.StatusMovedPermanently)
	})

	// Default client: allowRedirects=false → CheckRedirect returns RedirectError.
	c, err := client(false, false, false, 5000, false, "", "", "", false)
	if err != nil {
		t.Fatalf("client() err = %v", err)
	}

	respSize, dur, err := DoRequest(c, nil, "GET", "", ts.URL, "", nil)
	if err == nil {
		t.Fatalf("want err for blocked redirect, got nil (respSize=%d dur=%v)", respSize, dur)
	}
	if respSize != 0 {
		t.Errorf("respSize = %d, want 0", respSize)
	}
	if dur != 0 {
		t.Errorf("dur = %v, want 0", dur)
	}
}

// keepLastResponseClient returns an *http.Client that surfaces redirect responses
// without following them, so DoRequest's 301/307 branch is reachable.
func keepLastResponseClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func TestDoRequest_301RedirectAsResponse(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://example.invalid/")
		w.WriteHeader(http.StatusMovedPermanently)
	})

	respSize, dur, err := DoRequest(keepLastResponseClient(), nil, "GET", "", ts.URL, "", nil)
	if err != nil {
		t.Fatalf("DoRequest err = %v", err)
	}
	if respSize <= 0 {
		t.Errorf("respSize = %d, want > 0", respSize)
	}
	if dur <= 0 {
		t.Errorf("dur = %v, want > 0", dur)
	}
}

func TestDoRequest_307RedirectAsResponse(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://example.invalid/")
		w.WriteHeader(http.StatusTemporaryRedirect)
	})

	respSize, dur, err := DoRequest(keepLastResponseClient(), nil, "GET", "", ts.URL, "", nil)
	if err != nil {
		t.Fatalf("DoRequest err = %v", err)
	}
	if respSize <= 0 {
		t.Errorf("respSize = %d, want > 0", respSize)
	}
	if dur <= 0 {
		t.Errorf("dur = %v, want > 0", dur)
	}
}

func TestDoRequest_Non2xxIsError(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, _, err := DoRequest(defaultTestClient(t), nil, "GET", "", ts.URL, "", nil)
	if err == nil {
		t.Fatal("want err for 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "status code 500") {
		t.Errorf("err = %q, want substring %q", err.Error(), "status code 500")
	}
}

func TestDoRequest_BadURL(t *testing.T) {
	respSize, dur, err := DoRequest(defaultTestClient(t), nil, "GET", "", "://broken", "", nil)
	if err == nil {
		t.Fatal("want err for malformed URL, got nil")
	}
	if respSize != 0 {
		t.Errorf("respSize = %d, want 0", respSize)
	}
	if dur != 0 {
		t.Errorf("dur = %v, want 0", dur)
	}
}

func TestDoRequest_ServerClosesEarly(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Errorf("ResponseWriter is not a Hijacker")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Errorf("Hijack err = %v", err)
			return
		}
		_ = conn.Close()
	}))
	t.Cleanup(ts.Close)

	respSize, dur, err := DoRequest(defaultTestClient(t), nil, "GET", "", ts.URL, "", nil)
	if err == nil {
		t.Fatal("want err for connection closed mid-response, got nil")
	}
	if respSize != 0 {
		t.Errorf("respSize = %d, want 0", respSize)
	}
	if dur != 0 {
		t.Errorf("dur = %v, want 0", dur)
	}
}
