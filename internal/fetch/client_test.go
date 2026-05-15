package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetJSONSuccessAndHeaders(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("Accept header = %q, want application/json", got)
		}
		if got := r.Header.Get("User-Agent"); !strings.Contains(got, "tuip") {
			t.Fatalf("User-Agent header = %q, want it to contain tuip", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)

	var response struct {
		OK bool `json:"ok"`
	}
	if err := NewClient(5*time.Second).GetJSON(context.Background(), server.URL, &response); err != nil {
		t.Fatalf("GetJSON() error = %v", err)
	}
	if !response.OK {
		t.Fatalf("response.OK = false, want true")
	}
}

func TestGetTextSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); !strings.Contains(got, "text/html") {
			t.Fatalf("Accept header = %q, want text/html", got)
		}
		_, _ = w.Write([]byte(`hello`))
	}))
	t.Cleanup(server.Close)

	body, err := NewClient(5*time.Second).GetText(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("GetText() error = %v", err)
	}
	if body != "hello" {
		t.Fatalf("GetText() = %q, want hello", body)
	}
}

func TestGetJSONNon2xxReturnsError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	var response map[string]any
	err := NewClient(5*time.Second).GetJSON(context.Background(), server.URL, &response)
	if err == nil {
		t.Fatalf("GetJSON() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unexpected status") {
		t.Fatalf("GetJSON() error = %q, want unexpected status", err.Error())
	}
}

func TestGetJSONInvalidJSONReturnsError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not-json`))
	}))
	t.Cleanup(server.Close)

	var response map[string]any
	err := NewClient(5*time.Second).GetJSON(context.Background(), server.URL, &response)
	if err == nil {
		t.Fatalf("GetJSON() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Fatalf("GetJSON() error = %q, want decode", err.Error())
	}
}
