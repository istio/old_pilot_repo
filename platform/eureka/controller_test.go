package eureka

import (
	"testing"
	"time"

	"sync"

	"istio.io/pilot/model"
)

const (
	resync          = 5 * time.Millisecond
	notifyThreshold = resync * 10
)

type mockSyncClient struct {
	mutex sync.Mutex
	apps  []*application
}

func (m *mockSyncClient) Applications() ([]*application, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	apps := make([]*application, len(m.apps))
	copy(apps, m.apps)
	return apps, nil
}

func (m *mockSyncClient) SetApplications(apps []*application) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.apps = apps
}

var _ Client = (*mockSyncClient)(nil)

// TODO: ensure this test is reliable (no timing issues) on different systems
func TestController(t *testing.T) {
	cl := &mockSyncClient{}

	countMutex := sync.Mutex{}
	count := 0

	incrementCount := func() {
		countMutex.Lock()
		countMutex.Unlock()
		count++
	}
	getCountAndReset := func() int {
		countMutex.Lock()
		defer countMutex.Unlock()
		c := count
		count = 0
		return c
	}

	ctl := NewController(cl, resync)
	ctl.AppendInstanceHandler(func(instance *model.ServiceInstance, event model.Event) {
		incrementCount()
	})
	ctl.AppendServiceHandler(func(service *model.Service, event model.Event) {
		incrementCount()
	})

	stop := make(chan struct{})
	go ctl.Run(stop)
	defer close(stop)

	time.Sleep(notifyThreshold)
	if c := getCountAndReset(); c != 0 {
		t.Errorf("got %d notifications from controller, want %d", c, 0)
	}

	cl.SetApplications([]*application{
		{
			Name: "APP",
			Instances: []*instance{
				makeInstance("hello.world.local", "10.0.0.1", 8080, -1, nil),
				makeInstance("hello.world.local", "10.0.0.2", 8080, -1, nil),
			},
		},
	})
	time.Sleep(notifyThreshold)
	if c := getCountAndReset(); c != 2 {
		t.Errorf("got %d notifications from controller, want %d", count, 2)
	}
}
