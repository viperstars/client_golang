// Copyright 2021 The Prometheus Authors
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

package prometheus

import (
	"sort"
	"sync"

	dto "github.com/prometheus/client_model/go"
)

var _ rawCollector = &CachedCollector{}

// CachedCollector allows creating allocation friendly metrics which change less frequently than scrape time, yet
// label values can are changing over time. This collector
//
// If you happen to use NewDesc, NewConstMetric or MustNewConstMetric inside Collector.Collect routine, consider
// using CachedCollector instead.
type CachedCollector struct {
	descByFqName map[string]*Desc

	cacheIndexByName map[string]int
	cache            []*dto.MetricFamily
}

func (c *CachedCollector) Collect() []*dto.MetricFamily {
	return c.cache
}

// NewSession allows to collect all metrics in one go and update cache as much in-place
// as possible to save allocations.
func (c *CachedCollector) NewSession() *CollectSession {
	return &CollectSession{
		fqnames: make([]string, 0, len(c.descByFqName)),
	}
}

type CollectSession struct {
	c *CachedCollector

	fqnames []string
}

func (d *Desc) isContentEqual(help string, variableLabels []string, constLabels Labels) bool {
	if d.help != help {
		return false
	}
	if len(d.variableLabels) != len(variableLabels) {
		return false
	}
	if len(d.constLabelPairs) != len(constLabels) {
		return false
	}
	for i := range d.variableLabels {
		if d.variableLabels[i] != variableLabels[i] {
			return false
		}
	}
	for i := range d.constLabelPairs {
		v, ok := constLabels[*d.constLabelPairs[i].Name]
		if !ok || *d.constLabelPairs[i].Value != v {
			return false
		}
	}
	return true
}

func (s *CollectSession) Desc(fqName, help string, variableLabels []string, constLabels Labels) *Desc {
	s.fqnames = append(s.fqnames, fqName)
	if d, ok := s.c.descByFqName[fqName]; ok && d.isContentEqual(help, variableLabels, constLabels) {
		// Fast path if the same desc exists.
		return d
	}

	// No need to allocate all from scratch, we could replace and validate.
	s.c.descByFqName[fqName] = NewDesc(fqName, help, variableLabels, constLabels)
	return s.c.descByFqName[fqName]
}

type BlockingRegistry struct {
	Gatherer

	// rawCollector represents special collectors which requires blocking collect for the whole duration
	// of returned dto.MetricFamily usage.
	rawCollectors []rawCollector
	mu            sync.Mutex
}

func NewBlockingRegistry(g Gatherer) *BlockingRegistry {
	return &BlockingRegistry{
		Gatherer: g,
	}
}

type rawCollector interface {
	Collect() []*dto.MetricFamily
}

func (b *BlockingRegistry) RegisterRaw(r rawCollector) error {
	// TODO(bwplotka): Register, I guess for dups/check purposes?
	b.rawCollectors = append(b.rawCollectors, r)
	return nil
}

func (b *BlockingRegistry) Gather() (_ []*dto.MetricFamily, done func(), err error) {
	mfs, err := b.Gatherer.Gather()

	b.mu.Lock()
	// Returned mfs are sorted, so sort raw ones and inject
	// TODO(bwplotka): Implement concurrency for those?
	for _, r := range b.rawCollectors {
		// TODO(bwplotka): Check for duplicates.
		mfs = append(mfs, r.Collect()...)
	}

	// TODO(bwplotka): Consider sort in place, given metric family in gather is sorted already.
	sort.Slice(mfs, func(i, j int) bool {
		return *mfs[i].Name < *mfs[j].Name
	})
	return mfs, func() { b.mu.Unlock() }, err
}

// TransactionalGatherer ...
type TransactionalGatherer interface {
	// Gather ...
	Gather() (_ []*dto.MetricFamily, done func(), err error)
}
