package memory

import (
	"reflect"
	"sort"
	"time"

	"istio.io/pilot/model"
)

const ()

type Configs []model.Config

// Handler specifies a function to apply on a Config for a given event type
type Handler func(model.Config, model.Event)

type Monitor interface {
	Start(<-chan struct{})
	Stop()
	AppendEventHandler(string, Handler)
}

type configsMonitor struct {
	store              model.ConfigStore
	ticker             time.Ticker
	configCachedRecord map[string]Configs
	handlers           map[string][]Handler

	period    time.Duration
	stop      <-chan struct{}
	tickChan  <-chan time.Time
	isStopped bool
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
		handlers:   handlers,
		isStopped:          true,
	}
}

func (m *configsMonitor) Start(stop <-chan struct{}) {
	m.isStopped = false
	m.tickChan = time.NewTicker(m.period).C
	m.run(stop)
}

func (m *configsMonitor) Stop() {
	m.isStopped = true
}

func (m *configsMonitor) run(stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			m.Stop()
		case <-m.tickChan:
			if err := m.UpdateConfigRecord(); err != nil {
				m.Stop()
			}
		}
	}
}

func (m *configsMonitor) UpdateConfigRecord() error {
	for _, conf := range model.IstioConfigTypes {
		configs, err := m.store.List(conf.Type)
		if err != nil {
			return err
		}
		newRecord := Configs(configs)
		newRecord.normalize()
		if !reflect.DeepEqual(newRecord, m.configCachedRecord[conf.Type]) {
			// This is only a work-around solution currently
			// Since Handler functions generally act as a refresher
			// regardless of the input, thus passing in meaningless
			// input should make functionalities work
			obj := model.Config{}
			var event model.Event
			for _, f := range m.handlers[conf.Type] {
				f(obj, event)
			}
			m.configCachedRecord[conf.Type] = newRecord
		}
	}

	return nil
}

func (m *configsMonitor) AppendEventHandler(typ string, h Handler) {
	m.handlers[typ] = append(m.handlers[typ], h)
}

func (list Configs) normalize() {
	sort.Slice(list, func(i, j int) bool { return list[i].Key < list[j].Key })
}
