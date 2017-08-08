package memory

import (
	"reflect"
	"sort"
	"time"

	"istio.io/pilot/model"
)

type Configs []model.Config

// Handler specifies a function to apply on a Config for a given event type
type Handler func(model.Config, model.Event)

// AddHandler specifies a function to apply on a newly-added object
type AddHandler func(interface{})

// UpdateHandler specifies a function to apply on an updated object
type UpdateHandler func(interface{}, interface{})

// DeleteHandler specifies a function to apply on a deleted object
type DeleteHandler func(interface{})

type Monitor interface {
	Start(<-chan struct{})
	AppendEventHandler(string, Handler)
}

type configsMonitor struct {
	store              model.ConfigStore
	configCachedRecord map[string]Configs
	handlers           map[string][]Handler

	ticker   time.Ticker
	period   time.Duration
	tickChan <-chan time.Time

	stop <-chan struct{}
}

func NewConfigsMonitor(store model.ConfigStore, period time.Duration) Monitor {
	cache := make(map[string]Configs, 0)
	handlers := make(map[string][]Handler, 0)

	for _, conf := range model.IstioConfigTypes {
		cache[conf.Type] = make(Configs, 0)
		handlers[conf.Type] = make([]Handler, 0)
	}

	return &configsMonitor{
		store:              store,
		period:             period,
		configCachedRecord: cache,
		handlers:           handlers,
	}
}

func (m *configsMonitor) Start(stop <-chan struct{}) {
	m.tickChan = time.NewTicker(m.period).C
	m.run(stop)
}

func (m *configsMonitor) run(stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			m.tickChan.Stop()
		case <-m.tickChan:
			m.UpdateConfigRecord()
		}
	}
}

func (m *configsMonitor) UpdateConfigRecord() {
	for _, conf := range model.IstioConfigTypes {
		configs, err := m.store.List(conf.Type)
		if err != nil {
			glog.Warningf("Unable to fetch configs of type: %s", conf.Type)
			return
		}
		newRecord := Configs(configs)
		newRecord.normalize()
		m.compareToCache(conf.Type, m.configCachedRecord[conf.Type], newRecord)
		m.configCachedRecord[conf.Type] = newRecord
	}
}

func (m *configsMonitor) CompareToCache(typ string, oldRec, newRec Configs) {
	io, in := 0, 0
	for io < len(oldRec) && in < len(newRec) {
		if reflect.DeepEqual(oldRec[io], newRec[in]) {
		} else if oldRec[io].Key == newRec[in].Key {
			// An update event
			m.applyHandlers(typ, newRec[in], model.EventUpdate)
		} else if oldRec[io].Key < newRec[in].Key {
			// A delete event
			m.applyHandlers(typ, oldRec[io], model.EventDelete)
			in--
		} else {
			// An add event
			m.applyHandlers(typ, newRec[in], model.EventAdd)
			io--
		}
		io++
		in++
	}

	for ; io < len(oldRec); io++ {
		applyHandlers(typ, oldRec[io], model.EventDelete)
	}

	for ; in < len(newRec); in++ {
		applyHandlers(typ, newRec[in], model.EventAdd)
	}
}

func (m *configsMonitor) AppendEventHandler(typ string, h Handler) {
	m.handlers[typ] = append(m.handlers[typ], h)
}

func (m *configsMonitor) applyHandlers(typ string, config model.Config, e model.Event) {
	for f := range m.handlers[typ] {
		f(config, e)
	}
}

func (list Configs) normalize() {
	sort.Slice(list, func(i, j int) bool { return list[i].Key < list[j].Key })
}
