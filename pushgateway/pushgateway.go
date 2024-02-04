// Package pushgateway provides Prometheus Pushgateway API access.
package pushgateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Pushgateway reads and updates Pushgateway metrics.
type Pushgateway struct {
	baseURL    *url.URL
	httpClient *http.Client
}

// A group of metrics from the Query API.
type MetricsGroup struct {
	Labels  map[string]string
	Metrics Metrics
}

// The individual metrics in a MetricsGroup.
type Metrics map[string]Metric

// An individual metric in a Metrics map.
type Metric struct {
	Timestamp time.Time
}

func NewPushgateway(baseURL string) Pushgateway {
	url, err := url.Parse(baseURL)
	if err != nil {
		panic(fmt.Sprintf("Invalid baseURL: %v", err))
	}
	return Pushgateway{
		baseURL:    url,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Delete deletes the metrics for key using a [DELETE request].
//
// [DELETE request]: https://github.com/prometheus/pushgateway#delete-method
func (p Pushgateway) Delete(key string) error {
	if !strings.HasPrefix(key, "job/") {
		return fmt.Errorf("Key without job/ prefix.")
	}
	url, err := p.baseURL.Parse("/metrics/" + key)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("DELETE", url.String(), nil)
	if err != nil {
		return err
	}
	res, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 202 {
		return fmt.Errorf("HTTP response: %v", res.Status)
	}
	return nil
}

// QueryMetrics calls the [Query API] and returns MetricsGroup objects.
//
// [Query API]: https://github.com/prometheus/pushgateway#query-api
func (p Pushgateway) QueryMetrics() ([]MetricsGroup, error) {
	url, _ := p.baseURL.Parse("/api/v1/metrics")
	res, err := p.httpClient.Get(url.String())
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP response: %v", res.Status)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	var object map[string]interface{}
	err = json.Unmarshal(body, &object)
	if err != nil {
		return nil, err
	}
	groups, err := parseMetricsGroups(object)
	if err != nil {
		return nil, err
	}
	return groups, nil
}

// Upset pushes the up metric for key, using a [PUT request] when up is false.
//
// [PUT request]: https://github.com/prometheus/pushgateway#put-method
func (p Pushgateway) Upset(key string, up bool) error {
	if !strings.HasPrefix(key, "job/") {
		return fmt.Errorf("Key without job/ prefix.")
	}
	url, err := p.baseURL.Parse("/metrics/" + key)
	if err != nil {
		return err
	}
	var req *http.Request
	if up {
		req, err = http.NewRequest("POST", url.String(), strings.NewReader("up 1\n"))
	} else {
		req, err = http.NewRequest("PUT", url.String(), strings.NewReader("up 0\n"))
	}
	if err != nil {
		return err
	}
	res, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("HTTP response: %v", res.Status)
	}
	return nil
}

// TODO Base64 encoding
//
// Key returns a path containing label names and values, starting with "job".
// This can be used in Pushgateway [URLs].
//
// [URLs]: https://github.com/prometheus/pushgateway#url
func (g MetricsGroup) Key() string {
	job, ok := g.Labels["job"]
	if !ok {
		panic("Missing job label.")
	}
	names := make([]string, 0, len(g.Labels)-1)
	for name := range g.Labels {
		if name != "job" {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	parts := make([]string, 0, len(g.Labels)*2)
	parts = append(parts, "job", job)
	for _, name := range names {
		parts = append(parts, name, g.Labels[name])
	}
	return strings.Join(parts, "/")
}

// LabelNamesMatch returns true if the group label and provided names match.
func (g MetricsGroup) LabelNamesMatch(names ...string) bool {
	for _, name := range names {
		if _, ok := g.Labels[name]; !ok {
			return false
		}
	}
	return len(g.Labels) == len(names)
}

// Filter returns a copy of the Metrics map with the names removed.
func (m Metrics) Filter(names ...string) Metrics {
	result := Metrics{}
	for name, metric := range m {
		if !slices.Contains(names, name) {
			result[name] = metric
		}
	}
	return result
}

// MaxTimestamp returns the highest metric timestamp, or a zero Time.
func (m Metrics) MaxTimestamp() time.Time {
	if len(m) == 0 {
		return time.Time{}
	}
	timestamps := make([]time.Time, 0, len(m))
	for _, metric := range m {
		timestamps = append(timestamps, metric.Timestamp)
	}
	return slices.MaxFunc(timestamps, func(a, b time.Time) int { return a.Compare(b) })
}

// MinTimestamp returns the lowest metric timestamp, or a zero Time.
func (m Metrics) MinTimestamp() time.Time {
	if len(m) == 0 {
		return time.Time{}
	}
	timestamps := make([]time.Time, 0, len(m))
	for _, metric := range m {
		timestamps = append(timestamps, metric.Timestamp)
	}
	return slices.MinFunc(timestamps, func(a, b time.Time) int { return a.Compare(b) })
}

// parse... digs the relevant data out of the generic JSON object.

func parseMetricsGroups(object map[string]interface{}) ([]MetricsGroup, error) {
	status, ok := object["status"].(string)
	if !ok {
		panic(fmt.Sprintf("Invalid status attribute: %v", object["status"]))
	}
	if status != "success" {
		return nil, fmt.Errorf("Status attribute: %s", status)
	}
	data, ok := object["data"].([]interface{})
	if !ok {
		panic(fmt.Sprintf("Invalid data attribute: %v", object["data"]))
	}
	groups := make([]MetricsGroup, 0, len(data))
	for _, item := range data {
		group, ok := item.(map[string]interface{})
		if !ok {
			panic(fmt.Sprintf("Invalid data array item: %v", item))
		}
		groups = append(groups, parseMetricsGroup(group))
	}
	return groups, nil
}

func parseMetricsGroup(data map[string]interface{}) MetricsGroup {
	var labels map[string]string
	metrics := Metrics{}
	for k, v := range data {
		v_map, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if k == "labels" {
			labels = parseLabels(v_map)
		} else {
			metrics[k] = parseMetric(v_map)
		}
	}
	return MetricsGroup{Labels: labels, Metrics: metrics}
}

func parseLabels(data map[string]interface{}) map[string]string {
	labels := make(map[string]string, len(data))
	for k, v := range data {
		v_string, ok := v.(string)
		if !ok {
			panic(fmt.Sprintf("Invalid labels object: %v", data))
		}
		labels[k] = v_string
	}
	return labels
}

func parseMetric(data map[string]interface{}) Metric {
	timestamp_string, ok := data["time_stamp"].(string)
	if !ok {
		panic(fmt.Sprintf("Invalid time_stamp attribute: %v", data["time_stamp"]))
	}
	timestamp, err := time.Parse(time.RFC3339, timestamp_string)
	if err != nil {
		panic(fmt.Sprintf("Invalid time_stamp attribute: %v", err))
	}
	return Metric{Timestamp: timestamp}
}
