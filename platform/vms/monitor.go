package vms

import(
	"time"
	"sort"
	"reflect"
	"github.com/golang/glog"

	a8client "github.com/amalgam8/amalgam8/registry/client"
	a8api "github.com/amalgam8/amalgam8/pkg/api"
	"istio.io/pilot/model"
)

const()

type Services []*a8api.Service
type ServiceInstances []*a8api.ServiceInstance

// Handler specifies a function to apply on an object for a given event type
type Handler func(obj interface{}, event model.Event) error

type Monitor interface {
	Start(<-chan struct{})
	Stop()
	AppendServiceHandler(Handler)
	AppendInstanceHandler(Handler)
}

type vmsMonitor struct {
	discovery *a8client.Client
	ticker time.Ticker
	instanceCachedRecord ServiceInstances
	serviceCachedRecord Services
	instanceHandlers []Handler
	serviceHandlers []Handler
	period time.Duration
	stop <-chan struct{}
	tickChan <-chan time.Time
	isStopped bool
}

func NewVMsMonitor(client *a8client.Client, period time.Duration) Monitor {
	return &vmsMonitor{
		discovery: client,
		period: period,
		instanceCachedRecord: make(ServiceInstances, 0),
		serviceCachedRecord: make(Services, 0),
		instanceHandlers: make([]Handler, 0),
		serviceHandlers: make([]Handler, 0),
		isStopped: true,
	}
}

func (m *vmsMonitor) Start(stop <-chan struct{}) {
	m.isStopped = false
	m.tickChan = time.NewTicker(m.period).C
	m.run(stop)
}

func (m *vmsMonitor) Stop() {
	m.isStopped = true
}

func (m *vmsMonitor) run(stop <-chan struct{}) {
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

func (m *vmsMonitor) UpdateServiceRecord() error {
	svcs, err := m.discovery.ListServiceObjects()
	if err != nil {
		glog.Warningf("Error:%s in fetching service result", err)
		return err
	}
	newRecord := Services(svcs)
	newRecord.normalize()
	if !reflect.DeepEqual(newRecord, m.serviceCachedRecord) {
		// This is only a work-around solution currently
		// Since Handler functions generally act as a refresher 
		// regardless of the input, thus passing in meaningless 
		// input should make functionalities work
		obj := &a8api.Service{}
		var event model.Event
		for _, f := range m.serviceHandlers {
			if err := f(obj, event); err != nil{
				glog.Warningf("Error:%s in executing handler function", err)
				return err
			}
		}
		m.serviceCachedRecord = newRecord
	}
	return nil
}

func (m *vmsMonitor) UpdateInstanceRecord() error {
	insts, err := m.discovery.ListInstances()
	if err != nil {
		glog.Warningf("Error:%s in fetching instance result", err)
		return err
	}

	newRecord := ServiceInstances(insts)
	newRecord.normalize()
	if !reflect.DeepEqual(newRecord, m.instanceCachedRecord) {
		// This is only a work-around solution currently
		// Since Handler functions generally act as a refresher 
		// regardless of the input, thus passing in meaningless 
		// input should make functionalities work
		obj := &a8api.ServiceInstance{}
		var event model.Event
		for _, f := range m.instanceHandlers {
			if err := f(obj, event); err != nil{
				glog.Warningf("Error:%s in executing handler function", err)
				return err
			}
		}
		m.instanceCachedRecord = newRecord
	}
	return nil
}

func (m *vmsMonitor) AppendServiceHandler(h Handler) {
	m.serviceHandlers = append(m.serviceHandlers, h)
}

func (m *vmsMonitor) AppendInstanceHandler(h Handler) {
	m.instanceHandlers = append(m.instanceHandlers, h)
}

func (list Services) normalize() {
	sort.Slice(list, func(i, j int) bool {return list[i].ServiceName < list[j].ServiceName})
}

func (list ServiceInstances) normalize() {
	sort.Slice(list, func(i, j int) bool {return list[i].ID < list[j].ID})
}
