package memory

import (
	"time"

	"github.com/golang/protobuf/proto"
	"istio.io/pilot/model"
)

type controller struct {
	monitor     Monitor
	configStore model.ConfigStore
}

func NewController(cs model.ConfigStore) model.ConfigStoreCache {
	out := &controller{
		configStore: cs,
		monitor:     NewConfigsMonitor(cs, time.Second*1),
	}
	return out
}

func (c *controller) RegisterEventHandler(typ string, f func(model.Config, model.Event)) {
	c.monitor.AppendEventHandler(typ, f)
}

// Memory implementation is always synchronized with cache
func (c *controller) HasSynced() bool {
	return true
}

func (c *controller) Run(stop <-chan struct{}) {
	c.monitor.Start(stop)
}

func (c *controller) ConfigDescriptor() model.ConfigDescriptor {
	return c.configStore.ConfigDescriptor()
}

func (c *controller) Get(typ, key string) (proto.Message, bool, string) {
	return c.configStore.Get(typ, key)
}

func (c *controller) Post(val proto.Message) (string, error) {
	return c.configStore.Post(val)
}

func (c *controller) Put(val proto.Message, revision string) (string, error) {
	return c.configStore.Put(val, revision)
}

func (c *controller) Delete(typ, key string) error {
	return c.configStore.Delete(typ, key)
}

func (c *controller) List(typ string) ([]model.Config, error) {
	return c.configStore.List(typ)
}
