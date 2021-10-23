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
	"sync/atomic"

	"github.com/cespare/xxhash/v2"
	dto "github.com/prometheus/client_model/go"
)

// CachedCollector allows creating allocation friendly metrics which change less frequently than scrape time, yet
// label values can are changing over time. This collector
//
// If you happen to use NewDesc, NewConstMetric or MustNewConstMetric inside Collector.Collect routine, consider
// using CachedCollector instead.
type CachedCollector struct {
	descByFqName map[string]*Desc
}


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

func (s *CollectSession) NewDesc(fqName, help string, variableLabels []string, constLabels Labels) *Desc {
	s.fqnames = append(s.fqnames, fqName)
	if d, ok := s.c.descByFqName[fqName]; ok && d.isContentEqual(help, variableLabels, constLabels){
		// Fast path if the same desc exists.
		return d
	}

	// No need to allocate all from scratch, we could replace and validate.
	s.c.descByFqName[fqName]= NewDesc(fqName, help, variableLabels, constLabels)
	return s.c.descByFqName[fqName]
}

// MustNewCachedMetric is a version of NewCachedMetric that panics where
// NewCachedMetric would have returned an error.
func MustNewCachedMetric(desc *Desc, valueType ValueType, value float64, labelValues ...string) Metric {
	m, err := NewCachedMetric(desc, valueType, value, labelValues...)
	if err != nil {
		panic(err)
	}
	return m
}

// NewCachedMetric returns a metric ...
func NewCachedMetric(desc *Desc, valueType ValueType, value float64, labelValues ...string) (Metric, error) {
	m, err := NewConstMetric(desc, valueType, value, labelValues...)
	if err != nil {
		return nil, err
	}
	d := &dto.Metric{}
	if err := m.Write(d); err != nil {
		return nil, err
	}

	return &cachedMetric{
		desc: desc,
		metric: d,
	}, nil
}

var _ IteratableGatherer = &cachedGatherer{}

type cachedGatherer struct {
	g Gatherer

	pendingReaders atomic.Value
}

func newCachedGatherer(g Gatherer) *cachedGatherer {
	return &cachedGatherer{g:g }
}

// GatherIterate ...
func(g *cachedGatherer) GatherIterate() (MetricFamilyIterator, error) {
	mfs, err := g.g.Gather()
	if err != nil {
		return nil , err
	}



	return &mfIterator{mfs: mfs, i: -1}, err
}

// TODO(bwplotka): Consider making it public if useful.
type synchronizedCollector interface {
	SynchronizedCollect() *dto.MetricFamily

}

type SynchronizedRegistry struct {
	reg Gatherer
}

func

GatherIterate() (MetricFamilyIterator, error)
