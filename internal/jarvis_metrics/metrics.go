package jarvis_metrics

import (
	"context"
	"fmt"
	"integration-project-ehb/controlroom/pkg/logger"
	"io"
	"maps"
	"net/http"
	"strings"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

// Metrics represents a single metric point ready for indexing.
type Metrics struct {
	Timestamp time.Time      `json:"@timestamp"`
	Metric    string         `json:"metric"`
	Value     float64        `json:"value"`
	Labels    map[string]any `json:"labels"`
}

// RetrieveMetrics fetches Prometheus exposition format from mcp-master:8080/metrics,
// parses it, and yields MetricSample structs ready for indexing.
func RetrieveMetrics(ctx context.Context, client *http.Client, metricsUrl string) ([]Metrics, error) {
	logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, "[DEBUG] indexing metrics"))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/metrics", strings.TrimSuffix(metricsUrl, "/")), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metrics from mcp-master: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		errMsg := fmt.Sprintf("mcp-master returned %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("resposne code for fetching results %v", errMsg)
	}

	decoder := expfmt.NewDecoder(resp.Body, expfmt.FmtText)
	var samples []Metrics

	for {
		var mf dto.MetricFamily
		if err := decoder.Decode(&mf); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to decode metric family: %w", err)
		}

		// Skip if no metrics present
		if len(mf.GetMetric()) == 0 {
			continue
		}

		// note(nasr): ai genreated mapping stuff
		for _, metric := range mf.GetMetric() {
			labels := make(map[string]any)
			for _, labelPair := range metric.GetLabel() {
				labels[labelPair.GetName()] = labelPair.GetValue()
			}

			switch mf.GetType() {
			case dto.MetricType(0):
				if metric.Counter != nil {
					sample := Metrics{
						Timestamp: time.Now().UTC(),
						Metric:    mf.GetName(),
						Value:     metric.Counter.GetValue(),
						Labels:    labels,
					}
					samples = append(samples, sample)
				}

			case dto.MetricType(1):
				if metric.Gauge != nil {
					sample := Metrics{
						Timestamp: time.Now().UTC(),
						Metric:    mf.GetName(),
						Value:     metric.Gauge.GetValue(),
						Labels:    labels,
					}
					samples = append(samples, sample)
				}

			case dto.MetricType(2):
				if metric.Summary != nil {
					if metric.Summary.SampleSum != nil {
						sumSample := Metrics{
							Timestamp: time.Now().UTC(),
							Metric:    mf.GetName() + "_sum",
							Value:     metric.Summary.GetSampleSum(),
							Labels:    labels,
						}
						samples = append(samples, sumSample)
					}

					if metric.Summary.SampleCount != nil {
						countSample := Metrics{
							Timestamp: time.Now().UTC(),
							Metric:    mf.GetName() + "_count",
							Value:     float64(metric.Summary.GetSampleCount()),
							Labels:    labels,
						}
						samples = append(samples, countSample)
					}

					for _, quantile := range metric.Summary.GetQuantile() {
						quantileLabels := make(map[string]any)
						maps.Copy(quantileLabels, labels)
						quantileLabels["quantile"] = quantile.GetQuantile()

						quantileSample := Metrics{
							Timestamp: time.Now().UTC(),
							Metric:    mf.GetName(),
							Value:     quantile.GetValue(),
							Labels:    quantileLabels,
						}
						samples = append(samples, quantileSample)
					}
					continue
				}

			case dto.MetricType(3):
				if metric.Histogram != nil {
					if metric.Histogram.SampleCount != nil {
						countSample := Metrics{
							Timestamp: time.Now().UTC(),
							Metric:    mf.GetName() + "_count",
							Value:     float64(metric.Histogram.GetSampleCount()),
							Labels:    labels,
						}
						samples = append(samples, countSample)
					}

					if metric.Histogram.SampleSum != nil {
						sumSample := Metrics{
							Timestamp: time.Now().UTC(),
							Metric:    mf.GetName() + "_sum",
							Value:     metric.Histogram.GetSampleSum(),
							Labels:    labels,
						}
						samples = append(samples, sumSample)
					}

					for _, bucket := range metric.Histogram.GetBucket() {
						bucketLabels := make(map[string]any)
						maps.Copy(bucketLabels, labels)
						bucketLabels["le"] = bucket.GetUpperBound()

						bucketSample := Metrics{
							Timestamp: time.Now().UTC(),
							Metric:    mf.GetName() + "_bucket",
							Value:     float64(bucket.GetCumulativeCount()),
							Labels:    bucketLabels,
						}
						samples = append(samples, bucketSample)
					}
					continue
				}

			case dto.MetricType(5):
				if metric.Gauge != nil {
					sample := Metrics{
						Timestamp: time.Now().UTC(),
						Metric:    mf.GetName(),
						Value:     1.0,
						Labels:    labels,
					}
					samples = append(samples, sample)
				}

			}
		}
	}

	return samples, nil
}
