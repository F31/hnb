package storagemetrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Collector exports only bounded dimensions. Stable resource references remain
// in the read model and are intentionally not Prometheus labels.
type Collector struct {
	value *prometheus.Desc
	age   *prometheus.Desc
	read  func() []NormalizedSnapshot
}

func NewCollector(read func() []NormalizedSnapshot) *Collector {
	labels := []string{"provider", "metric", "unit", "source", "freshness", "applicability"}
	return &Collector{
		value: prometheus.NewDesc("hnb_storage_metric_value", "Normalized provider storage metric value.", labels, nil),
		age:   prometheus.NewDesc("hnb_storage_metric_age_seconds", "Age of the provider storage observation.", labels, nil),
		read:  read,
	}
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) { ch <- c.value; ch <- c.age }

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	for _, snapshot := range c.read() {
		for _, metric := range snapshot.Metrics {
			if metric.Status != Known || metric.Value == nil {
				continue
			}
			labels := []string{snapshot.ProviderID, string(metric.Kind), metric.Unit, metric.Source, string(metric.Freshness), string(metric.Applicability)}
			ch <- prometheus.MustNewConstMetric(c.value, prometheus.GaugeValue, *metric.Value, labels...)
			age := time.Since(metric.ObservedAt).Seconds()
			if age < 0 {
				age = 0
			}
			ch <- prometheus.MustNewConstMetric(c.age, prometheus.GaugeValue, age, labels...)
		}
	}
}
