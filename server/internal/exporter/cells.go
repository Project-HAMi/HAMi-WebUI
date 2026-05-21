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
// landed between reset() and the end of populate saw partial / empty data, which
// surfaces in the UI as "vGPU 分配率有时有数据，有时没有数据" — series flickering
// in and out at scrape boundaries.
//
// The fix: instead of wiping the GaugeVec at the start of each cycle, every Set
// goes through MetricsGenerator.set, which both writes the value AND records the
// (gauge, label tuple) it touched. After a cycle completes successfully, we walk
// the previous cycle's recorded set and DeleteLabelValues for any tuple that
// disappeared this round. Existing series are atomically overwritten in place,
// brand-new series appear when their Set runs, vanished series disappear at the
// end-of-cycle prune. There is no window where a known device is missing.

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
}

// labelSep is a 0-byte separator that cannot appear in normal Prometheus label
// values, so strings.Join produces an unambiguous key.
const labelSep = "\x00"

// set writes value into the gauge and records the (gauge, labels) tuple in the
// current-cycle map. Safe for concurrent use; the collector itself only calls
// it from one goroutine, but we lock anyway in case callers add a parallel pass.
func (s *MetricsGenerator) set(g *prometheus.GaugeVec, value float64, labels ...string) {
	g.WithLabelValues(labels...).Set(value)

	k := cellKey{gauge: g, joined: strings.Join(labels, labelSep)}
	s.cellMu.Lock()
	defer s.cellMu.Unlock()
	if s.current == nil {
		s.current = make(map[cellKey]cell)
	}
	// Copy the labels slice — callers reuse the underlying array between iterations.
	s.current[k] = cell{gauge: g, labels: append([]string(nil), labels...)}
}

// commitCycle promotes the current cycle to "previous" and removes any label
// tuple that existed in the previous cycle but not this one. Call ONLY when the
// cycle completed without being cut short by ctx cancellation: pruning on a
// partial map would erroneously delete cells that just weren't re-Set this time.
func (s *MetricsGenerator) commitCycle() {
	s.cellMu.Lock()
	defer s.cellMu.Unlock()
	if s.current == nil {
		// Nothing was written this cycle; leave prev intact.
		return
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
}

// dropCurrentCycle discards the in-progress map without promoting it. Use this
// when a cycle ran into ctx cancellation or any other partial-completion path,
// so the next cycle's prune still references the last KNOWN-GOOD snapshot.
//
// Any partial Set() calls that did land remain in the GaugeVec as freshly
// overwritten cells — that is intentional and harmless, since they only update
// values on label tuples that already existed.
func (s *MetricsGenerator) dropCurrentCycle() {
	s.cellMu.Lock()
	s.current = nil
	s.cellMu.Unlock()
}
