package golb

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"time"
)

type InstanceInfo struct {
	ID                    int64
	Name                  string
	Alive                 bool
	URL                   *url.URL
	HealthCheckURL        *url.URL
	LastUTCTimestsamp     *time.Time
	LastErrorUTCTimestamp *time.Time
	LastError             error
	LastInstanceStatus    *InstanceStatus
}

type InstanceStatus struct {
	Status  bool                   `json:"status"`
	Details map[string]interface{} `json:"details"`
}

func (instance *InstanceInfo) RunHealthCheckInterval(ctx context.Context, interval time.Duration) {
	client := &http.Client{
		Timeout: time.Second * 5,
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			log.Printf("Running health check for instance: %s", instance.Name)
			instance.runHealthCheck(ctx, client)
		case <-ctx.Done():
			log.Printf("Stopping health check for instance: %s", instance.Name)
			return
		}
	}
}

func (instance *InstanceInfo) runHealthCheck(ctx context.Context, client *http.Client) {
	timestamp := time.Now().UTC()

	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, instance.HealthCheckURL.String(), nil)
	if err != nil {
		instance.updateStatus(false, &timestamp, err)
		return
	}

	res, err := client.Do(req)
	if err != nil || res.StatusCode != http.StatusOK {
		instance.updateStatus(false, &timestamp, err)
		return
	}
	defer res.Body.Close()

	instanceStatus := &InstanceStatus{}
	err = json.NewDecoder(res.Body).Decode(instanceStatus)
	if err != nil {
		instance.updateStatus(false, &timestamp, err)
		return
	}

	instance.updateStatus(instanceStatus.Status, &timestamp, nil)
	instance.LastInstanceStatus = instanceStatus
}

func (instance *InstanceInfo) updateStatus(alive bool, timestamp *time.Time, err error) {
	instance.Alive = alive
	instance.LastUTCTimestsamp = timestamp

	if err != nil {
		log.Printf("Health check failed for %s: %v", instance.Name, err)

		instance.LastError = err
		instance.LastErrorUTCTimestamp = timestamp
	}
}
