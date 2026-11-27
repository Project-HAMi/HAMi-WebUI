package exporter

import (
	"github.com/prometheus/client_golang/prometheus"

	"vgpu/internal/resourceview"
)

const (
	splitReasonOverflow = iota
	splitReasonSeriesLimit
	splitReasonTimeout
)

var (
	promQueryOutcomes = []string{"ok", "error", "circuit_open", "response_too_large", "canceled"}
	batchSplitReasons = [...]string{
		splitReasonOverflow:    "response_too_large",
		splitReasonSeriesLimit: "series_limit",
		splitReasonTimeout:     "timeout",
	}
	observationStates = []string{
		resourceview.Known,
		resourceview.Stale,
		resourceview.Unknown,
		resourceview.Conflict,
	}
)

type selfObservabilityCollector struct {
	generator *MetricsGenerator

	promQueries      *prometheus.Desc
	breakerOpens     *prometheus.Desc
	breakerOpen      *prometheus.Desc
	batchSplits      *prometheus.Desc
	observationState *prometheus.Desc
}

func newSelfObservabilityCollector(generator *MetricsGenerator) *selfObservabilityCollector {
	return &selfObservabilityCollector{
		generator: generator,
		promQueries: prometheus.NewDesc(
			"hami_webui_prom_queries_total",
			"Total bounded Prometheus queries grouped by outcome.",
			[]string{"outcome"}, nil,
		),
		breakerOpens: prometheus.NewDesc(
			"hami_webui_prom_circuit_breaker_opens_total",
			"Total transitions of the Prometheus query circuit breaker to the open state.",
			nil, nil,
		),
		breakerOpen: prometheus.NewDesc(
			"hami_webui_prom_circuit_breaker_open",
			"Whether the Prometheus query circuit breaker is currently open or half-open (1) vs closed (0).",
			nil, nil,
		),
		batchSplits: prometheus.NewDesc(
			"hami_webui_telemetry_batch_splits_total",
			"Total telemetry batches split into smaller fallback chunks, grouped by trigger reason.",
			[]string{"reason"}, nil,
		),
		observationState: prometheus.NewDesc(
			"hami_webui_telemetry_observations",
			"Number of published physical-device observation fields by freshness/verification state.",
			[]string{"state"}, nil,
		),
	}
}

func (c *selfObservabilityCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.promQueries
	ch <- c.breakerOpens
	ch <- c.breakerOpen
	ch <- c.batchSplits
	ch <- c.observationState
}

func (c *selfObservabilityCollector) Collect(ch chan<- prometheus.Metric) {
	if c.generator == nil {
		return
	}
	stats := c.generator.promClient.Stats()
	outcomeValues := map[string]uint64{
		"ok":                 stats.QueriesOK,
		"error":              stats.QueriesError,
		"circuit_open":       stats.QueriesRejectedCircuit,
		"response_too_large": stats.QueriesTooLarge,
		"canceled":           stats.QueriesCanceled,
	}
	for _, outcome := range promQueryOutcomes {
		ch <- prometheus.MustNewConstMetric(c.promQueries, prometheus.CounterValue, float64(outcomeValues[outcome]), outcome)
	}
	ch <- prometheus.MustNewConstMetric(c.breakerOpens, prometheus.CounterValue, float64(stats.CircuitOpens))
	breakerOpenVal := 0.0
	if stats.CircuitOpen {
		breakerOpenVal = 1.0
	}
	ch <- prometheus.MustNewConstMetric(c.breakerOpen, prometheus.GaugeValue, breakerOpenVal)

	for i, reason := range batchSplitReasons {
		ch <- prometheus.MustNewConstMetric(c.batchSplits, prometheus.CounterValue, float64(c.generator.splitCounts[i].Load()), reason)
	}

	stateCounts := map[string]int{}
	for _, obs := range c.generator.DeviceObservations() {
		for _, q := range []resourceview.Quantity{obs.UtilizationRatio, obs.MemoryUsedBytes, obs.MemoryPhysicalBytes} {
			if st := q.State.GetState(); st != "" {
				stateCounts[st]++
			}
		}
	}
	for _, state := range observationStates {
		ch <- prometheus.MustNewConstMetric(c.observationState, prometheus.GaugeValue, float64(stateCounts[state]), state)
	}
}
