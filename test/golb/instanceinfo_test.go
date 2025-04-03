package golb_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/prafitradimas/golb/pkg/golb"
)

const healthPath = "/health"

type instanceServer struct {
	URL       *url.URL
	HealthURL *url.URL
	Server    *httptest.Server
	Wg        sync.WaitGroup
	*testing.T
}

func newInstanceServer(t *testing.T, handler http.HandlerFunc) *instanceServer {
	server := httptest.NewServer(handler)

	u, err := url.Parse(server.URL)
	if err != nil {
		panic(err)
	}

	instance := instanceServer{T: t}
	instance.URL = u
	instance.HealthURL = u.JoinPath(healthPath)
	instance.Server = server

	return &instance
}

func (i *instanceServer) Close() {
	i.Wg.Wait()
	i.Server.Close()
}

func TestRunHealthCheckInterval(t *testing.T) {
	instanceServer := newInstanceServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == healthPath {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": true}`))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	instanceServer.Wg.Add(1)
	defer func() {
		instanceServer.Wg.Done()
		instanceServer.Close()
	}()

	instance := &golb.InstanceInfo{
		ID:             1,
		Name:           "Test",
		URL:            instanceServer.URL,
		HealthCheckURL: instanceServer.HealthURL,
		Alive:          true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		instance.RunHealthCheckInterval(ctx, time.Millisecond*100)
	}()

	time.Sleep(time.Millisecond * 250)
	cancel()

	if !instance.Alive {
		t.Fatal("Expected instance to be alive, but it's marked as dead")
	}

	if instance.LastInstanceStatus == nil || !instance.LastInstanceStatus.Status {
		t.Fatal("Expected last instance status to be healthy")
	}
}

func TestRunHealthCheckInterval_Down(t *testing.T) {
	instanceServer := newInstanceServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	instanceServer.Wg.Add(1)
	defer func() {
		instanceServer.Wg.Done()
		instanceServer.Close()
	}()

	instance := &golb.InstanceInfo{
		ID:             1,
		Name:           "Test",
		URL:            instanceServer.URL,
		HealthCheckURL: instanceServer.HealthURL,
		Alive:          false,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		instance.RunHealthCheckInterval(ctx, time.Millisecond*100)
	}()

	time.Sleep(time.Millisecond * 250)
	cancel()

	if instance.Alive {
		t.Fatal("Expected instance to be dead, but it's marked as alive")
	}

	if instance.LastInstanceStatus != nil && instance.LastInstanceStatus.Status {
		t.Fatal("Expected last instance status to be dead")
	}
}

func TestRunHealthCheckInterval_Timeout(t *testing.T) {
	instanceServer := newInstanceServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Second * 5)
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	instanceServer.Wg.Add(1)
	defer func() {
		instanceServer.Wg.Done()
		instanceServer.Close()
	}()

	instance := &golb.InstanceInfo{
		ID:             1,
		Name:           "Test",
		URL:            instanceServer.URL,
		HealthCheckURL: instanceServer.HealthURL,
		Alive:          true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		instance.RunHealthCheckInterval(ctx, time.Millisecond*100)
	}()

	time.Sleep(time.Millisecond * 250)
	cancel()

	// Wait for the in-flight health check to complete its update.
	time.Sleep(time.Millisecond * 100)

	if instance.Alive {
		t.Fatal("Expected instance to be dead, but it's marked as alive")
	}

	if instance.LastInstanceStatus != nil && instance.LastInstanceStatus.Status {
		t.Fatal("Expected last instance status to be dead")
	}

	if instance.LastError == nil {
		t.Fatal("Expected LastError to be set due to timeout")
	}

	if instance.LastErrorUTCTimestamp == nil {
		t.Fatal("Expected LastErrorUTCTimestamp to be set due to timeout")
	}
}
