package httpclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestClientRetriesGetOnServerError(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&hits, 1)
		if current < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":"slow down"}`)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()

	client := New(server.URL, "perm:token", server.Client(), true, io.Discard)
	payload, err := client.DoJSON(context.Background(), http.MethodGet, "/api/issues", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(payload.(map[string]any)["ok"]) != "true" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if atomic.LoadInt32(&hits) != 3 {
		t.Fatalf("expected 3 attempts, got %d", hits)
	}
}

func TestClientDebugLoggingDoesNotLeakQueryString(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()

	stderr := &bytes.Buffer{}
	client := New(server.URL, "perm:token", server.Client(), true, stderr)
	_, err := client.DoJSON(context.Background(), http.MethodGet, "/api/issues", url.Values{"query": []string{"summary:secret customer"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stderr.String(), "secret") || strings.Contains(stderr.String(), "query=") {
		t.Fatalf("query leaked in debug output: %s", stderr.String())
	}
}
