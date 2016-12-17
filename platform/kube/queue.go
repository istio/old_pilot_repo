// Copyright 2016 Google Inc.
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

package kube

import (
	"log"
	"sync"
	"time"

	"k8s.io/client-go/pkg/util/flowcontrol"
)

// Queue of work tickets processed using a rate-limiting loop
type Queue interface {
	// Push a ticket
	Push(Task)
	// Run the loop until a signal on the channel
	Run(chan struct{})
}

// Task object for the event watchers; processes until handler succeeds
type Task struct {
	handler func(obj interface{}) error
	obj     interface{}
}

type queueImpl struct {
	delay   time.Duration
	queue   []Task
	lock    sync.Mutex
	closing bool
}

// NewQueue instantiates a queue with a processing function
func NewQueue() Queue {
	return &queueImpl{
		delay:   1 * time.Second,
		queue:   make([]Task, 0),
		closing: false,
		lock:    sync.Mutex{},
	}
}

func (q *queueImpl) Push(item Task) {
	q.lock.Lock()
	if !q.closing {
		q.queue = append(q.queue, item)
	}
	q.lock.Unlock()
}

func (q *queueImpl) Run(stop chan struct{}) {
	go func() {
		<-stop
		q.lock.Lock()
		q.closing = true
		q.lock.Unlock()
	}()

	// Throttle processing up to smoothed 10 qps with bursts up to 100 qps
	rateLimiter := flowcontrol.NewTokenBucketRateLimiter(float32(10), 100)
	var item Task
	for {
		rateLimiter.Accept()

		q.lock.Lock()
		if q.closing {
			q.lock.Unlock()
			return
		} else if len(q.queue) == 0 {
			q.lock.Unlock()
		} else {
			item, q.queue = q.queue[0], q.queue[1:]
			q.lock.Unlock()

			for {
				err := item.handler(item.obj)
				if err != nil {
					log.Println("Work item failed, repeating after delay:", err)
					time.Sleep(q.delay)
				} else {
					break
				}
			}
		}
	}
}
