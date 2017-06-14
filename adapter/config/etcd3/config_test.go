package etcd3

import (
	"istio.io/pilot/test/mock"
	"testing"
	"istio.io/pilot/model"
	"github.com/coreos/etcd/clientv3"
	"context"
	"fmt"
)

func makeClient(t *testing.T) model.ConfigStore {
	// FIXME: should not have k8s dependency
	cl, err := clientv3.New(clientv3.Config{
		Endpoints: []string{"http://192.168.99.100:32379"},
	})
	if err != nil {
		t.Error(err)
	}

	// FIXME: temp until store is cleaned every run
	resp, err := cl.Delete(context.Background(), "\x00") // FIXME: is this the clean way to nuke everything?
	if err != nil {
		t.Error(err)
	}
	fmt.Println(resp.Deleted)

	return NewClient(cl, mock.Types)
}

func TestEtcd3(t *testing.T) {
	cl := makeClient(t)
	mock.CheckMapInvariant(cl, t, 10)
}
