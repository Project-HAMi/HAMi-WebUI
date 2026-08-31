package exporter

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// Diff-based cell tracking for the background /metrics collector.
//
// Why this exists: the old reset()+populate cycle (called from a synchronous HTTP
// handler) was safe because scrape only ever observed the fully-populated state.
// Once the cycle moved to a background goroutine, every Prometheus scrape that
// landed between reset() and the end of populate saw partial / empty data. The
// resulting series flickered in and out of the UI at scrape boundaries.
//
// The fix: instead of wiping or mutating GaugeVecs while upstream queries are in
// flight, every Set stages the value and label tuple in memory. A completed cycle
// applies the staged values and removes tuples that disappeared. A fatal cycle can
// therefore discard its staging map without leaking partial values or new labels.

// cellKey identifies a single observation (gauge vector + concrete label tuple).
// The joined string is just a map-key encoding of the labels; the original
// []string is kept on the cell so we can pass it to DeleteLabelValues.
type cellKey struct {
	gauge  *prometheus.GaugeVec
	joined string
}

type cell struct {
	gauge  *prometheus.GaugeVec
	labels []string
	value  float64
}

// labelSep is a 0-byte separator that cannot appear in normal Prometheus label
// values, so strings.Join produces an unambiguous key.
const labelSep = "\x00"

func (s *MetricsGenerator) beginCycle() {
	s.cellMu.Lock()
	s.current = make(map[cellKey]cell)
	s.degradedCount = 0
	s.firstDegraded = nil
	s.fatalErr = nil
	s.cellMu.Unlock()
}

// set stages a value and its label tuple in the current-cycle map. GaugeVecs are
// only changed by commitCycle, so dropCurrentCycle can leave the last committed
// registry snapshot untouched.
func (s *MetricsGenerator) set(g *prometheus.GaugeVec, value float64, labels ...string) {
	k := cellKey{gauge: g, joined: strings.Join(labels, labelSep)}
	s.cellMu.Lock()
	defer s.cellMu.Unlock()
	if s.current == nil {
		s.current = make(map[cellKey]cell)
	}
	// Copy the labels slice — callers reuse the underlying array between iterations.
	s.current[k] = cell{gauge: g, labels: append([]string(nil), labels...), value: value}
}

// commitCycle promotes the current cycle to "previous" and removes any label
// tuple that existed in the previous cycle but not this one. It may commit a
// best-effort degraded cycle, but must only be called after the full inventory
// walk completes: pruning a fatally partial map would erase unvisited cells.
func (s *MetricsGenerator) commitCycle() {
	s.cellMu.Lock()
	defer s.cellMu.Unlock()
	if s.current == nil {
		// Nothing was written this cycle; leave prev intact.
		s.degradedCount = 0
		s.firstDegraded = nil
		s.fatalErr = nil
		return
	}
	for _, c := range s.current {
		c.gauge.WithLabelValues(c.labels...).Set(c.value)
	}

	deleted := 0
	for k, c := range s.prev {
		if _, ok := s.current[k]; ok {
			continue
		}
		if c.gauge.DeleteLabelValues(c.labels...) {
			deleted++
		}
	}
	if deleted > 0 {
		s.log.Debugw("msg", "pruned stale metric cells", "count", deleted)
	}
	s.prev = s.current
	s.current = nil
	s.degradedCount = 0
	s.firstDegraded = nil
	s.fatalErr = nil
}

// dropCurrentCycle discards the in-progress map without promoting it. Use this
// when a cycle ran into ctx cancellation or any other partial-completion path,
// so the next cycle's prune still references the last committed snapshot.
func (s *MetricsGenerator) dropCurrentCycle() {
	s.cellMu.Lock()
	s.current = nil
	s.degradedCount = 0
	s.firstDegraded = nil
	s.fatalErr = nil
	s.cellMu.Unlock()
}
