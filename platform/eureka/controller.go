package eureka

import (
	"time"

	"reflect"

	"github.com/golang/glog"
	"istio.io/pilot/model"
)

type serviceHandler func(*model.Service, model.Event)
type instanceHandler func(*model.ServiceInstance, model.Event)

type controller struct {
	interval         time.Duration
	serviceHandlers  []serviceHandler
	instanceHandlers []instanceHandler
	client           Client
}

func NewController(client Client) model.Controller {
	return &controller{
		interval:         1 * time.Second,
		serviceHandlers:  make([]serviceHandler, 0),
		instanceHandlers: make([]instanceHandler, 0),
		client:           client,
	}
}

func (c *controller) AppendServiceHandler(f func(*model.Service, model.Event)) error {
	c.serviceHandlers = append(c.serviceHandlers, f)
	return nil
}

func (c *controller) AppendInstanceHandler(f func(*model.ServiceInstance, model.Event)) error {
	c.instanceHandlers = append(c.instanceHandlers, f)
	return nil
}

func (c *controller) Run(stop <-chan struct{}) {
	var cachedApps []*Application
	ticker := time.NewTicker(c.interval)
	for {
		select {
		case <-ticker.C:
			apps, err := c.client.Applications()
			if err != nil {
				glog.Warningf("Periodic Eureka poll failed: %v", err)
				continue
			}

			if !reflect.DeepEqual(apps, cachedApps) {
				cachedApps = apps
				// TODO: feed with real events.
				// The handlers are being feed dummy events. This is sufficient with simplistic handlers
				// that invalidate the cache on any event but will not work with smarter handlers.
				for _, h := range c.serviceHandlers {
					go h(&model.Service{}, model.EventAdd)
				}
				for _, h := range c.instanceHandlers {
					go h(&model.ServiceInstance{}, model.EventAdd)
				}
			}
		case <-stop:
			ticker.Stop()
			break
		}
	}
}
