// Copyright 2017 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package tpr provides an implementation of the config store and cache
// using Kubernetes Third-Party Resources and the informer framework from Kubernetes
package etcd3

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/coreos/etcd/clientv3"
	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/hashicorp/go-multierror"
	"istio.io/pilot/model"
)

// NewClient creates an etcd3 client that implements the store interface
func NewClient(cl *clientv3.Client, descriptor model.ConfigDescriptor) model.ConfigStore {
	return &client{
		descriptor: descriptor,
		client:     cl,
	}
}

// TODO: global context so that requests are close on a Close()?
// TODO: close client?
type client struct {
	descriptor model.ConfigDescriptor
	client     *clientv3.Client
}

// ConfigDescriptor implements store interface
func (cl *client) ConfigDescriptor() model.ConfigDescriptor {
	return cl.descriptor
}

// Get implements store interface
func (cl *client) Get(typ, key string) (proto.Message, bool, string) {
	schema, exists := cl.descriptor.GetByType(typ)
	if !exists {
		glog.Warning("missing type %q", typ)
		return nil, false, ""
	}

	resp, err := cl.client.Get(context.Background(), formatKey(typ, key))
	if err != nil {
		glog.Warning(err)
		return nil, false, ""
	}

	if len(resp.Kvs) < 1 {
		return nil, false, ""
	}

	out, err := schema.FromJSON(string(resp.Kvs[0].Value))
	if err != nil {
		glog.Warning(err)
		return nil, false, ""
	}
	return out, true, fmt.Sprintf("%d", resp.Kvs[0].Version)
}

// List implements store interface
func (cl *client) List(typ string) ([]model.Config, error) {
	schema, exists := cl.descriptor.GetByType(typ)
	if !exists {
		return nil, fmt.Errorf("missing type %q", typ)
	}

	resp, err := cl.client.Get(context.Background(), formatKey(typ, ""), clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	var errs error
	out := make([]model.Config, 0)
	for _, kv := range resp.Kvs {
		content, err := schema.FromJSON(string(kv.Value))
		if err != nil {
			multierror.Append(errs, err)
			continue
		}
		_, key := parseKey(kv.Key)
		out = append(out, model.Config{
			Type:     typ,
			Key:      key,
			Revision: fmt.Sprintf("%d", kv.Version),
			Content:  content,
		})
	}
	return out, errs
}

// Delete implements store interface
func (cl *client) Delete(typ, key string) error {
	if _, exists := cl.descriptor.GetByType(typ); !exists {
		return fmt.Errorf("missing type %q", typ)
	}
	resp, err := cl.client.Delete(context.Background(), formatKey(typ, key))
	if err != nil {
		return err
	}
	if resp.Deleted != 1 {
		return &model.ItemNotFoundError{Key: key}
	}
	return nil
}

func (cl *client) buildKeyValue(config proto.Message) (typ, key, value string, err error) {
	schema, ok := cl.descriptor.GetByMessageName(proto.MessageName(config))
	if !ok {
		err = fmt.Errorf("missing type %q", typ)
		return
	}
	if err = schema.Validate(config); err != nil {
		return
	}

	typ = schema.Type
	key = schema.Key(config)
	value, err = schema.ToJSON(config)
	return
}

// Post implements store interface
func (cl *client) Post(config proto.Message) (string, error) {
	typ, key, value, err := cl.buildKeyValue(config)
	if err != nil {
		return "", err
	}

	// create the key-value pair if it does not already exist.
	txn := cl.client.Txn(context.Background())
	resp, err := txn.If(
		clientv3.Compare(clientv3.Version(formatKey(typ, key)), "=", 0),
	).Then(
		clientv3.OpPut(formatKey(typ, key), value),
	).Commit()
	if err != nil {
		return "", err
	}
	if !resp.Succeeded {
		return "", &model.ItemAlreadyExistsError{Key: key}
	}

	// key-value pair was just created, so the version will be 1.
	return "1", nil
}

// Put implements store interface
func (cl *client) Put(config proto.Message, oldRevision string) (string, error) {
	typ, key, value, err := cl.buildKeyValue(config)
	if err != nil {
		return "", err
	}
	rev, err := strconv.ParseInt(oldRevision, 10, 64)
	if err != nil {
		return "", multierror.Prefix(err, "invalid revision")
	}

	// update the key-value pair if it exists.
	txn := cl.client.Txn(context.Background())
	resp, err := txn.If(
		clientv3.Compare(clientv3.Version(formatKey(typ, key)), "=", rev),
	).Then(
		clientv3.OpPut(formatKey(typ, key), value),
	).Commit()
	if err != nil {
		return "", err
	}
	if !resp.Succeeded {
		return "", &model.ItemNotFoundError{Key: key}
	}

	// version will be incremented.
	return fmt.Sprintf("%d", rev+1), nil
}

func formatKey(typ, key string) string {
	return fmt.Sprintf("%v/%v/%v", "istio", typ, key)
}

func parseKey(data []byte) (string, string) {
	parts := strings.Split(string(data), "/")
	if len(parts) < 2 {
		return "", ""
	}

	return parts[1], parts[2]
}
