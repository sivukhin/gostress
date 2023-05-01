package gostress

import (
	"github.com/prometheus/client_golang/prometheus"
	"strings"
)

var (
	ExpectedRpsGauge      = prometheus.NewGauge(prometheus.GaugeOpts{})
	ExpectedWorkersGauge  = prometheus.NewGauge(prometheus.GaugeOpts{})
	CurrentWorkersGauge   = prometheus.NewGauge(prometheus.GaugeOpts{})
	SentRequestCounter    = prometheus.NewCounter(prometheus.CounterOpts{})
	SkippedRequestCounter = prometheus.NewCounter(prometheus.CounterOpts{})
	ErrorsCounter         = prometheus.NewCounter(prometheus.CounterOpts{})
	RequestLatency        = prometheus.NewHistogramVec(prometheus.HistogramOpts{}, []string{"status"})
)

func registerMetrics(name string) {
	tokens := strings.SplitN(name, "/", 2)
	labels := prometheus.Labels{"group": "gostress", "gostress_name": name, "gostress_category": tokens[0]}
	ExpectedRpsGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "gostress_expected_rps",
		Help:        "gostress expected rps",
		ConstLabels: labels,
	})
	prometheus.MustRegister(ExpectedRpsGauge)

	ExpectedWorkersGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "gostress_expected_workers",
		Help:        "gostress expected workers",
		ConstLabels: labels,
	})
	prometheus.MustRegister(ExpectedWorkersGauge)

	CurrentWorkersGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "gostress_current_workers",
		Help:        "gostress current workers",
		ConstLabels: labels,
	})
	prometheus.MustRegister(CurrentWorkersGauge)

	SentRequestCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "gostress_sent_request_counter",
		Help:        "gostress sent request counter",
		ConstLabels: labels,
	})
	prometheus.MustRegister(SentRequestCounter)

	SkippedRequestCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "gostress_skipped_request_counter",
		Help:        "gostress skipped request counter",
		ConstLabels: labels,
	})
	prometheus.MustRegister(SkippedRequestCounter)

	ErrorsCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "gostress_errors_request_counter",
		Help:        "gostress errors request counter",
		ConstLabels: labels,
	})
	prometheus.MustRegister(ErrorsCounter)

	RequestLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "gostress_request_latency",
		Help:        "gostress request latency",
		ConstLabels: labels,
		Buckets:     []float64{0.001, 0.005, 0.01, 0.02, 0.05, 0.1, 0.2, 0.3, 0.5, 0.7, 1.0, 5.0},
	}, []string{"status"})
	prometheus.MustRegister(RequestLatency)
}
