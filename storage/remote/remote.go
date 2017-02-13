// Copyright 2017 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package remote

import (
	"sync"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
)

// Storage allows queueing samples for remote writes.
type Storage struct {
	mtx            sync.RWMutex
	externalLabels model.LabelSet
	queues         []*StorageQueueManager
}

// ApplyConfig updates the state as the new config requires.
func (s *Storage) ApplyConfig(conf *config.Config) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	newQueues := []*StorageQueueManager{}
	// TODO: we should only stop & recreate queues which have changes,
	// as this can be quite disruptive.
	for i, conf := range conf.RemoteWriteConfigs {
		c, err := NewClient(i, conf)
		if err != nil {
			return err
		}
		newQueues = append(newQueues, NewStorageQueueManager(StorageQueueManagerConfig{
			Client:         c,
			RelabelConfigs: conf.WriteRelabelConfigs,
		}))
	}

	for _, q := range s.queues {
		q.Stop()
	}

	s.queues = newQueues
	s.externalLabels = conf.GlobalConfig.ExternalLabels
	for _, q := range s.queues {
		q.Start()
	}
	return nil
}

// Stop the background processing of the storage queues.
func (s *Storage) Stop() {
	for _, q := range s.queues {
		q.Stop()
	}
}

// Append implements storage.SampleAppender. Always returns nil.
func (s *Storage) Append(smpl *model.Sample) error {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	for _, q := range s.queues {
		var snew model.Sample
		snew = *smpl
		snew.Metric = smpl.Metric.Clone()

		for ln, lv := range s.externalLabels {
			if _, ok := smpl.Metric[ln]; !ok {
				snew.Metric[ln] = lv
			}
		}
		q.Append(&snew)
	}
	return nil
}

// NeedsThrottling implements storage.SampleAppender. It will always return
// false as a remote storage drops samples on the floor if backlogging instead
// of asking for throttling.
func (s *Storage) NeedsThrottling() bool {
	return false
}
