package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// VMetricsClient is an HTTP client for the VictoriaMetrics Prometheus-compatible API.
// Endpoints: /api/v1/query, /api/v1/query_range, /api/v1/labels,
// /api/v1/label/{name}/values, /api/v1/series, /api/v1/metadata
type VMetricsClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewVMetricsClient creates a client for a VictoriaMetrics instance.
func NewVMetricsClient(baseURL string) *VMetricsClient {
	return &VMetricsClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// InstantQuery executes a PromQL instant query at an optional point in time.
func (c *VMetricsClient) InstantQuery(ctx context.Context, query string, evalTime *string) (json.RawMessage, error) {
	params := url.Values{"query": {query}}
	if evalTime != nil && *evalTime != "" {
		params.Set("time", *evalTime)
	}
	return c.get(ctx, "/api/v1/query", params)
}

// RangeQuery executes a PromQL range query over a time interval.
func (c *VMetricsClient) RangeQuery(ctx context.Context, query, start, end, step string) (json.RawMessage, error) {
	params := url.Values{
		"query": {query},
		"start": {start},
		"end":   {end},
		"step":  {step},
	}
	return c.get(ctx, "/api/v1/query_range", params)
}

// LabelNames returns all label names, optionally filtered by time range.
func (c *VMetricsClient) LabelNames(ctx context.Context, match []string) (json.RawMessage, error) {
	params := url.Values{}
	for _, m := range match {
		params.Add("match[]", m)
	}
	return c.get(ctx, "/api/v1/labels", params)
}

// LabelValues returns values for a specific label name.
func (c *VMetricsClient) LabelValues(ctx context.Context, label string) (json.RawMessage, error) {
	path := fmt.Sprintf("/api/v1/label/%s/values", url.PathEscape(label))
	return c.get(ctx, path, nil)
}

// Series returns series matching the provided selectors.
func (c *VMetricsClient) Series(ctx context.Context, matchers []string, start, end string) (json.RawMessage, error) {
	params := url.Values{}
	for _, m := range matchers {
		params.Add("match[]", m)
	}
	if start != "" {
		params.Set("start", start)
	}
	if end != "" {
		params.Set("end", end)
	}
	return c.get(ctx, "/api/v1/series", params)
}

// Metadata returns metric metadata, optionally filtered by metric name.
func (c *VMetricsClient) Metadata(ctx context.Context, metric *string) (json.RawMessage, error) {
	params := url.Values{}
	if metric != nil && *metric != "" {
		params.Set("metric", *metric)
	}
	return c.get(ctx, "/api/v1/metadata", params)
}

// DeleteSeries deletes time series matching the provided selector.
// VictoriaMetrics admin endpoint: POST /api/v1/admin/tsdb/delete_series
func (c *VMetricsClient) DeleteSeries(ctx context.Context, match string) error {
	params := url.Values{"match[]": {match}}
	u := c.baseURL + "/api/v1/admin/tsdb/delete_series?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("VictoriaMetrics delete_series error (HTTP %d): %s", resp.StatusCode, truncate(string(body), 512))
	}
	return nil
}

// Snapshot creates a TSDB snapshot for backup purposes.
// VictoriaMetrics endpoint: GET /snapshot/create
func (c *VMetricsClient) Snapshot(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, "/snapshot/create", nil)
}

// get performs a GET request and returns the raw JSON body.
func (c *VMetricsClient) get(ctx context.Context, path string, params url.Values) (json.RawMessage, error) {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("VictoriaMetrics API error (HTTP %d): %s", resp.StatusCode, truncate(string(body), 512))
	}

	return json.RawMessage(body), nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
