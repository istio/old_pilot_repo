package consul

import (
	"github.com/golang/glog"
	"reflect"
	"sort"
	"time"

	"github.com/hashicorp/consul/api"
	"istio.io/pilot/model"
)

const ()

type ConsulServices map[string][]string
type ConsulServiceInstances []*api.CatalogService

// Handler specifies a function to apply on an object for a given event type
type Handler func(obj interface{}, event model.Event) error

type Monitor interface {
	Start(<-chan struct{})
	Stop()
	AppendServiceHandler(Handler)
	AppendInstanceHandler(Handler)
}

type consulMonitor struct {
	discovery            *api.Client
	ticker               time.Ticker
	instanceCachedRecord ConsulServiceInstances
	serviceCachedRecord  ConsulServices
	instanceHandlers     []Handler
	serviceHandlers      []Handler
	period               time.Duration
	stop                 <-chan struct{}
	tickChan             <-chan time.Time
	isStopped            bool
}

func NewConsulMonitor(client *api.Client, period time.Duration) Monitor {
	return &consulMonitor{
		discovery:            client,
		period:               period,
		instanceCachedRecord: make(ConsulServiceInstances, 0),
		serviceCachedRecord:  make(ConsulServices, 0),
		instanceHandlers:     make([]Handler, 0),
		serviceHandlers:      make([]Handler, 0),
		isStopped:            true,
	}
}

func (m *consulMonitor) Start(stop <-chan struct{}) {
	m.isStopped = false
	m.tickChan = time.NewTicker(m.period).C
	m.run(stop)
}

func (m *consulMonitor) Stop() {
	m.isStopped = true
}

func (m *consulMonitor) run(stop <-chan struct{}) {
	var err error
	for {
		select {
		case <-stop:
			m.Stop()
		case <-m.tickChan:
			if err = m.UpdateServiceRecord(); err != nil {
				m.Stop()
			}
			if err = m.UpdateInstanceRecord(); err != nil {
				m.Stop()
			}
		}
	}
}

func (m *consulMonitor) UpdateServiceRecord() error {
	svcs, _, err := m.discovery.Catalog().Services(nil)
	if err != nil {
		glog.Warningf("Error:%s in fetching service result", err)
		return err
	}
	newRecord := ConsulServices(svcs)
	if !reflect.DeepEqual(newRecord, m.serviceCachedRecord) {
		// This is only a work-around solution currently
		// Since Handler functions generally act as a refresher
		// regardless of the input, thus passing in meaningless
		// input should make functionalities work
		//TODO
		obj := &[]*api.CatalogService{}
		var event model.Event
		for _, f := range m.serviceHandlers {
			if err := f(obj, event); err != nil {
				glog.Warningf("Error:%s in executing handler function", err)
				return err
			}
		}
		m.serviceCachedRecord = newRecord
	}
	return nil
}

func (m *consulMonitor) UpdateInstanceRecord() error {
	svcs, _, err := m.discovery.Catalog().Services(nil)
	if err != nil {
		glog.Warningf("Error:%s in fetching instance result", err)
		return err
	}

	insts := []*api.CatalogService{}
	for name, _ := range svcs {
		endpoints, _, err := m.discovery.Catalog().Service(name, "", nil)
		if err != nil {
			glog.Warningf("Could not retrieve service catalogue from consul: %v", err)
			continue
		}
		insts = append(insts, endpoints...)

	}

	newRecord := ConsulServiceInstances(insts)
	newRecord.normalize()
	if !reflect.DeepEqual(newRecord, m.instanceCachedRecord) {
		// This is only a work-around solution currently
		// Since Handler functions generally act as a refresher
		// regardless of the input, thus passing in meaningless
		// input should make functionalities work
		obj := &api.CatalogService{}
		var event model.Event
		for _, f := range m.instanceHandlers {
			if err := f(obj, event); err != nil {
				glog.Warningf("Error:%s in executing handler function", err)
				return err
			}
		}
		m.instanceCachedRecord = newRecord
	}
	return nil
}

func (m *consulMonitor) AppendServiceHandler(h Handler) {
	m.serviceHandlers = append(m.serviceHandlers, h)
}

func (m *consulMonitor) AppendInstanceHandler(h Handler) {
	m.instanceHandlers = append(m.instanceHandlers, h)
}

func (list ConsulServiceInstances) normalize() {
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
}
