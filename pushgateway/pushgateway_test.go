package pushgateway

import (
	_ "embed"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var now = time.Now()
var later = now.Add(time.Second)
var earlier = now.Add(-time.Second)

var group = MetricsGroup{
	Labels: map[string]string{"job": "foo", "instance": "bar"},
	Metrics: map[string]Metric{
		"baz":                       Metric{Timestamp: now},
		"push_time_seconds":         Metric{Timestamp: later},
		"push_failure_time_seconds": Metric{Timestamp: earlier},
	},
}

func TestMetricsGroup(t *testing.T) {
	expectedKey, actualKey := "job/foo/instance/bar", group.Key()
	if actualKey != expectedKey {
		t.Errorf("Expected key: %v, got: %v", expectedKey, actualKey)
	}

	matchingLabelNames := []string{"instance", "job"}
	if !group.LabelNamesMatch(matchingLabelNames...) {
		t.Errorf("Expected label names to match: %v", matchingLabelNames)
	}
	tooFewLabelNames := []string{"job"}
	if group.LabelNamesMatch(tooFewLabelNames...) {
		t.Errorf("Expected label names to not match: %v", tooFewLabelNames)
	}
	tooManyLabelNames := []string{"instance", "job", "qux"}
	if group.LabelNamesMatch(tooManyLabelNames...) {
		t.Errorf("Expected label names to not match: %v", tooManyLabelNames)
	}
}

func TestMetricsMinTimestamp(t *testing.T) {
	if timestamp := group.Metrics.MinTimestamp(); timestamp != earlier {
		t.Errorf("Expected timestamp: %v, got: %v", earlier, timestamp)
	}
	metrics := group.Metrics.Filter("push_time_seconds", "push_failure_time_seconds")
	if timestamp := metrics.MinTimestamp(); timestamp != now {
		t.Errorf("Expected timestamp: %v, got: %v", now, timestamp)
	}
	metrics = group.Metrics.Filter("baz", "push_time_seconds", "push_failure_time_seconds")
	if timestamp := metrics.MinTimestamp(); !timestamp.IsZero() {
		t.Errorf("Expected zero timestamp, got: %v", timestamp)
	}

}

func TestMetricsMaxTimestamp(t *testing.T) {
	if timestamp := group.Metrics.MaxTimestamp(); timestamp != later {
		t.Errorf("Expected timestamp: %v, got: %v", later, timestamp)
	}
	metrics := group.Metrics.Filter("push_time_seconds", "push_failure_time_seconds")
	if timestamp := metrics.MaxTimestamp(); timestamp != now {
		t.Errorf("Expected timestamp: %v, got: %v", now, timestamp)
	}
	metrics = group.Metrics.Filter("baz", "push_time_seconds", "push_failure_time_seconds")
	if timestamp := metrics.MaxTimestamp(); !timestamp.IsZero() {
		t.Errorf("Expected zero timestamp, got: %v", timestamp)
	}
}

func TestPushgatewayDelete(t *testing.T) {
	var method, body string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics/job/test" {
			method = r.Method
			bytes, err := io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(400)
			} else {
				body = string(bytes)
			}
			w.WriteHeader(202)
		} else {
			t.Logf("Expected metrics path, got: %v", r.URL.Path)
			w.WriteHeader(400)
		}
	}))
	defer ts.Close()

	client := NewPushgateway(ts.URL)

	err := client.Delete("job/test")
	if err != nil {
		t.Fatalf("Expected nil error, got: %v", err)
	}
	if method != "DELETE" {
		t.Fatalf("Expected DELETE method, got: %v", method)
	}
	if body != "" {
		t.Fatalf("Expected empty body, got: %v", body)
	}

	err = client.Delete("foo/bar")
	if err == nil {
		t.Fatalf("Expected non-nil error.")
	}
}

//go:embed testdata/metrics.json
var metrics []byte

//go:embed testdata/metrics-error.json
var metricsError []byte

func TestPushgatewayQueryMetrics(t *testing.T) {
	body := metrics

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/metrics" {
			w.Header().Set("Content-Type", "application/json")
			w.Write(body)
		} else {
			t.Logf("Expected Query API path, got: %v", r.URL.Path)
			w.WriteHeader(400)
		}
	}))
	defer ts.Close()

	client := NewPushgateway(ts.URL)
	groups, err := client.QueryMetrics()
	if err != nil {
		t.Fatalf("Expected nil error, got: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("Expected one MetricsGroup, got: %v", len(groups))
	}

	group := groups[0]
	if !group.LabelNamesMatch("job", "instance") {
		t.Errorf("Expected job/instance labels, got: %v", group.Labels)
	}
	metrics := group.Metrics.Filter("push_time_seconds", "push_failure_time_seconds")
	actualTimestamp := metrics.MinTimestamp().Unix()
	if actualTimestamp != 0 {
		t.Errorf("Expected timestamp = 0, got: %v", actualTimestamp)
	}

	body = metricsError
	_, err = client.QueryMetrics()
	if err == nil {
		t.Errorf("Expected non-nil error.")
	}
}

func TestPushgatewayUpset(t *testing.T) {
	var method, body string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics/job/test" {
			method = r.Method
			bytes, err := io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(400)
			} else {
				body = string(bytes)
			}
		} else {
			t.Logf("Expected metrics path, got: %v", r.URL.Path)
			w.WriteHeader(400)
		}
	}))
	defer ts.Close()

	client := NewPushgateway(ts.URL)

	err := client.Upset("job/test", true)
	if err != nil {
		t.Fatalf("Expected nil error, got: %v", err)
	}
	if method != "POST" {
		t.Fatalf("Expected POST method, got: %v", method)
	}
	if body != "up 1\n" {
		t.Fatalf("Expected 'up 1\\n' body, got: %v", body)
	}

	err = client.Upset("job/test", false)
	if err != nil {
		t.Fatalf("Expected nil error, got: %v", err)
	}
	if method != "PUT" {
		t.Fatalf("Expected PUT method, got: %v", method)
	}
	if body != "up 0\n" {
		t.Fatalf("Expected 'up 0\\n' body, got: %v", body)
	}

	err = client.Upset("foo/bar", true)
	if err == nil {
		t.Fatalf("Expected non-nil error.")
	}
}
