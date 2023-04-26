package gostress

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"go.uber.org/zap"
	"strings"
	"time"
)

func quantile(q float64, histogram *io_prometheus_client.Histogram) float64 {
	count := q * float64(histogram.GetSampleCount())
	lowerBound := float64(0)
	for _, bucket := range histogram.GetBucket() {
		if count > float64(bucket.GetCumulativeCount()) {
			lowerBound = bucket.GetUpperBound()
			continue
		}
		r := 1.0 - (float64(bucket.GetCumulativeCount())-count)/float64(bucket.GetCumulativeCount())
		return lowerBound + (bucket.GetUpperBound()-lowerBound)*r
	}
	return lowerBound
}

func PrintMetric(name string, metrics []*io_prometheus_client.Metric) string {
	type m struct{ name, value string }
	lines := make([]m, 0)
	for _, metric := range metrics {
		var current m
		labels := make([]string, 0)
		for _, label := range metric.GetLabel() {
			if label.GetName() == "gostress_name" || label.GetName() == "gostress_category" || label.GetValue() == "gostress" {
				continue
			}
			labels = append(labels, fmt.Sprintf("%v:%v", label.GetName(), label.GetValue()))
		}
		if len(labels) > 0 {
			current.name = fmt.Sprintf("{%v}", strings.Join(labels, ","))
		}
		if counter := metric.GetCounter(); counter != nil {
			current.value = fmt.Sprintf("count=%v", counter.GetValue())
		} else if gauge := metric.GetGauge(); gauge != nil {
			current.value = fmt.Sprintf("value=%v", gauge.GetValue())
		} else if histogram := metric.GetHistogram(); histogram != nil {
			current.value = fmt.Sprintf(
				"count=%v, avg=%.4f, p50=%.4f, p90=%.4f, p99=%.4f",
				histogram.GetSampleCount(),
				histogram.GetSampleSum()/float64(histogram.GetSampleCount()),
				quantile(0.50, histogram),
				quantile(0.90, histogram),
				quantile(0.99, histogram),
			)
		}
		lines = append(lines, current)
	}
	if len(lines) == 1 && lines[0].name == "" {
		return fmt.Sprintf("%32v: %v", name, lines[0].value)
	} else {
		padded := make([]string, 0, len(lines))
		for _, line := range lines {
			padded = append(padded, fmt.Sprintf("%32v: %v", line.name, line.value))
		}
		return fmt.Sprintf("%32v:\n%v", name, strings.Join(padded, "\n"))
	}
}

func PrintStat() (string, error) {
	metrics, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return "", err
	}
	stat := strings.Builder{}
	for _, metric := range metrics {
		name := metric.GetName()
		if strings.HasPrefix(name, "go_") || strings.HasPrefix(name, "process_") {
			continue
		}
		stat.WriteString(fmt.Sprintf("%v\n", PrintMetric(name, metric.GetMetric())))
	}
	return stat.String(), nil
}

func Monitor(name string, interval time.Duration, logger *zap.SugaredLogger) func() {
	startTime := time.Now()
	finish := make(chan struct{})
	go func() {
		for {
			select {
			case <-time.NewTimer(interval).C:
				stat, err := PrintStat()
				if err != nil {
					logger.Errorf("unable to gather metrics: %v", err)
				} else {
					logger.Infof("stress test stat (%v, elapsed %v)\n%v", name, time.Since(startTime), stat)
				}
			case <-finish:
				stat, err := PrintStat()
				if err != nil {
					logger.Errorf("unable to gather metrics: %v", err)
				} else {
					logger.Infof("stress test stat (%v, final)\n%v", name, stat)
				}
				return
			}
		}
	}()
	return func() { finish <- struct{}{} }
}
