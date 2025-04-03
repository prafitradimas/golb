package golb

import (
	"net/url"
	"sync"
	"time"
)

type Registry interface {
	Register(name string, rawURL string) error
	GetInstanceInfoByName(name string) InstanceInfo
	GetInstanceInfoByNameAndId(id int64, name string) InstanceInfo
}

type instanceRegistry struct {
	mu sync.RWMutex

	instances           map[string][]*InstanceInfo // grouped by name
	healthCheckInterval time.Duration
}

func (registry *instanceRegistry) Register(id int64, name string, rawURL string) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}

	instance := &InstanceInfo{
		ID:   id,
		Name: name,
		URL:  u,
	}

	if instances, ok := registry.instances[name]; ok {
		instances = append(instances, instance)
	} else {
		registry.instances[name] = []*InstanceInfo{instance}
	}

	return nil
}
