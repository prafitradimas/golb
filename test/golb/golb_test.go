package golb_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prafitradimas/golb/pkg/golb"
)

// Test that the ServerPool.Next method correctly returns a healthy server using round-robin.
func TestServerPool_Next(t *testing.T) {
	sp := &golb.ServerPool{}

	// Create two backend test servers.
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("server1"))
	}))
	defer ts1.Close()
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("server2"))
	}))
	defer ts2.Close()

	// Add servers to the pool.
	if err := sp.AddServer("server1", ts1.URL); err != nil {
		t.Fatalf("Error adding server1: %v", err)
	}
	if err := sp.AddServer("server2", ts2.URL); err != nil {
		t.Fatalf("Error adding server2: %v", err)
	}

	// Mark server1 as alive and server2 as down.
	sp.Servers[0].SetAlive(true)
	sp.Servers[1].SetAlive(false)

	peer := sp.Next()
	if peer == nil || peer.Name != "server1" {
		t.Errorf("Expected server1 as next healthy server, got: %+v", peer)
	}

	// Mark both as alive and verify round-robin selection.
	sp.Servers[1].SetAlive(true)
	count := make(map[string]int)
	for i := 0; i < 10; i++ {
		peer = sp.Next()
		count[peer.Name]++
	}
	if count["server1"] == 0 || count["server2"] == 0 {
		t.Errorf("Round robin not working properly, distribution: %+v", count)
	}
}

// Test that the ServerPool.ServeHTTP method forwards the request to one of the healthy backends.
func TestServerPool_ServeHTTP(t *testing.T) {
	sp := &golb.ServerPool{}

	// Create two backend test servers.
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("server1"))
	}))
	defer ts1.Close()
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("server2"))
	}))
	defer ts2.Close()

	// Add servers to the pool.
	if err := sp.AddServer("server1", ts1.URL); err != nil {
		t.Fatalf("Error adding server1: %v", err)
	}
	if err := sp.AddServer("server2", ts2.URL); err != nil {
		t.Fatalf("Error adding server2: %v", err)
	}

	// Mark both servers as alive.
	sp.Servers[0].SetAlive(true)
	sp.Servers[1].SetAlive(true)

	req := httptest.NewRequest("GET", "/", nil)
	res := httptest.NewRecorder()

	sp.ServeHTTP(res, req)
	resp := res.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	body := res.Body.String()
	if body != "server1" && body != "server2" {
		t.Errorf("Unexpected response body: %s", body)
	}
}

// Test that the error handler eventually calls the fallback after exceeding max retries.
// This test uses an unreachable backend URL to force connection errors.
func TestServer_ErrorHandler_Fallback(t *testing.T) {
	sp := &golb.ServerPool{}
	// Use an address that is very likely unreachable.
	badURL := "http://127.0.0.1:1"
	if err := sp.AddServer("badServer", badURL); err != nil {
		t.Fatalf("Error adding bad server: %v", err)
	}
	sp.Servers[0].SetAlive(true)

	req := httptest.NewRequest("GET", "/", nil)
	res := httptest.NewRecorder()

	// Call the reverse proxy directly. The unreachable URL should trigger the ErrorHandler.
	sp.Servers[0].ReverseProxy.ServeHTTP(res, req)

	// Allow some extra time for retries to occur.
	time.Sleep(50 * time.Millisecond)

	resp := res.Result()
	// When max attempts are reached the FallbackHandler sends a 503.
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d after retries, got %d", http.StatusServiceUnavailable, resp.StatusCode)
	}
}
