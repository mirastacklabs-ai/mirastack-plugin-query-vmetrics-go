package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	mirastack "github.com/mirastacklabs-ai/mirastack-agents-sdk-go"
	"github.com/mirastacklabs-ai/mirastack-agents-sdk-go/datetimeutils"
)

// ── isValidVMTimeParam ───────────────────────────────────────────────────────

func TestIsValidVMTimeParam(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"", false},
		{"-", false},
		{"+", false},
		{"  ", false},
		{" - ", false},
		{" + ", false},
		{"-1h", true},
		{"now", true},
		{"1743379200", true},
		{"1743379200.000", true},
		{"-30m", true},
		{"2026-04-10T00:00:00Z", true},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("input=%q", tc.input), func(t *testing.T) {
			got := isValidVMTimeParam(tc.input)
			if got != tc.valid {
				t.Errorf("isValidVMTimeParam(%q) = %v, want %v", tc.input, got, tc.valid)
			}
		})
	}
}

// ── actionRangeQuery ─────────────────────────────────────────────────────────

func TestActionRangeQuery_UsesTimeRange(t *testing.T) {
	var capturedStart, capturedEnd string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedStart = r.URL.Query().Get("start")
		capturedEnd = r.URL.Query().Get("end")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))
	defer srv.Close()

	p := &QueryVMetricsPlugin{client: NewVMetricsClient(srv.URL)}

	tr := &mirastack.TimeRange{
		StartEpochMs: 1743379200000, // 2025-03-31T00:00:00Z
		EndEpochMs:   1743382800000, // 2025-03-31T01:00:00Z
	}

	_, err := p.actionRangeQuery(context.Background(), map[string]string{
		"query": "up",
		"step":  "1m",
	}, tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedStart := datetimeutils.FormatEpochSeconds(tr.StartEpochMs)
	expectedEnd := datetimeutils.FormatEpochSeconds(tr.EndEpochMs)

	if capturedStart != expectedStart {
		t.Errorf("expected start=%s, got %s", expectedStart, capturedStart)
	}
	if capturedEnd != expectedEnd {
		t.Errorf("expected end=%s, got %s", expectedEnd, capturedEnd)
	}
}

func TestActionRangeQuery_FallbackDefaultsWhenParamsEmpty(t *testing.T) {
	var capturedStart, capturedEnd string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedStart = r.URL.Query().Get("start")
		capturedEnd = r.URL.Query().Get("end")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))
	defer srv.Close()

	p := &QueryVMetricsPlugin{client: NewVMetricsClient(srv.URL)}

	// No TimeRange, no start/end params → defaults to 1h window using NowUTCMs
	_, err := p.actionRangeQuery(context.Background(), map[string]string{
		"query": "up",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify non-empty epoch-seconds values were sent (not "-" or "now")
	if capturedStart == "" || capturedStart == "-" {
		t.Errorf("expected valid start, got %q", capturedStart)
	}
	if capturedEnd == "" || capturedEnd == "-" {
		t.Errorf("expected valid end, got %q", capturedEnd)
	}
}

func TestActionRangeQuery_FallbackRejectsInvalidDash(t *testing.T) {
	var capturedStart string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedStart = r.URL.Query().Get("start")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))
	defer srv.Close()

	p := &QueryVMetricsPlugin{client: NewVMetricsClient(srv.URL)}

	// start="-" is the exact bug scenario — should fall back to default
	_, err := p.actionRangeQuery(context.Background(), map[string]string{
		"query": "up",
		"start": "-",
		"end":   "now",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedStart == "-" {
		t.Error("start=\"-\" should have been replaced with a valid epoch-seconds default")
	}
	if capturedStart == "" {
		t.Error("start should not be empty")
	}
}

func TestActionRangeQuery_FallbackAcceptsValidRelative(t *testing.T) {
	var capturedStart, capturedEnd string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedStart = r.URL.Query().Get("start")
		capturedEnd = r.URL.Query().Get("end")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))
	defer srv.Close()

	p := &QueryVMetricsPlugin{client: NewVMetricsClient(srv.URL)}

	// Valid relative params should pass through untouched
	_, err := p.actionRangeQuery(context.Background(), map[string]string{
		"query": "up",
		"start": "-1h",
		"end":   "now",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedStart != "-1h" {
		t.Errorf("expected start=-1h, got %q", capturedStart)
	}
	if capturedEnd != "now" {
		t.Errorf("expected end=now, got %q", capturedEnd)
	}
}

func TestActionRangeQuery_MissingQuery(t *testing.T) {
	p := &QueryVMetricsPlugin{client: NewVMetricsClient("http://localhost:0")}

	_, err := p.actionRangeQuery(context.Background(), map[string]string{}, nil)
	if err == nil {
		t.Fatal("expected error for missing query")
	}
}

func TestActionRangeQuery_DefaultStep(t *testing.T) {
	var capturedStep string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedStep = r.URL.Query().Get("step")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))
	defer srv.Close()

	p := &QueryVMetricsPlugin{client: NewVMetricsClient(srv.URL)}

	tr := &mirastack.TimeRange{StartEpochMs: 1743379200000, EndEpochMs: 1743382800000}
	_, err := p.actionRangeQuery(context.Background(), map[string]string{
		"query": "up",
		// no step → should default to "1m"
	}, tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedStep != "1m" {
		t.Errorf("expected default step=1m, got %q", capturedStep)
	}
}

// ── actionInstantQuery ───────────────────────────────────────────────────────

func TestActionInstantQuery_UsesTimeRange(t *testing.T) {
	var capturedTime string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTime = r.URL.Query().Get("time")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer srv.Close()

	p := &QueryVMetricsPlugin{client: NewVMetricsClient(srv.URL)}

	tr := &mirastack.TimeRange{
		StartEpochMs: 1743379200000,
		EndEpochMs:   1743382800000,
	}

	_, err := p.actionInstantQuery(context.Background(), map[string]string{
		"query": "up",
	}, tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := datetimeutils.FormatEpochSeconds(tr.EndEpochMs)
	if capturedTime != expected {
		t.Errorf("expected time=%s, got %s", expected, capturedTime)
	}
}

// ── actionSeries ─────────────────────────────────────────────────────────────

func TestActionSeries_UsesTimeRange(t *testing.T) {
	var capturedStart, capturedEnd string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedStart = r.URL.Query().Get("start")
		capturedEnd = r.URL.Query().Get("end")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":[]}`))
	}))
	defer srv.Close()

	p := &QueryVMetricsPlugin{client: NewVMetricsClient(srv.URL)}

	tr := &mirastack.TimeRange{
		StartEpochMs: 1743379200000,
		EndEpochMs:   1743382800000,
	}

	_, err := p.actionSeries(context.Background(), map[string]string{
		"match": "up",
	}, tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedStart != datetimeutils.FormatEpochSeconds(tr.StartEpochMs) {
		t.Errorf("expected start epoch seconds, got %q", capturedStart)
	}
	if capturedEnd != datetimeutils.FormatEpochSeconds(tr.EndEpochMs) {
		t.Errorf("expected end epoch seconds, got %q", capturedEnd)
	}
}

func TestActionSeries_FallbackRejectsInvalidDash(t *testing.T) {
	var capturedStart, capturedEnd string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedStart = r.URL.Query().Get("start")
		capturedEnd = r.URL.Query().Get("end")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":[]}`))
	}))
	defer srv.Close()

	p := &QueryVMetricsPlugin{client: NewVMetricsClient(srv.URL)}

	_, err := p.actionSeries(context.Background(), map[string]string{
		"match": "up",
		"start": "-",
		"end":   "-",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Invalid "-" should be cleared to empty string (series endpoint allows empty start/end)
	if capturedStart == "-" {
		t.Error("start=\"-\" should have been rejected")
	}
	if capturedEnd == "-" {
		t.Error("end=\"-\" should have been rejected")
	}
}

// ── Execute (dispatch routing) ───────────────────────────────────────────────

func TestExecute_MissingAction(t *testing.T) {
	p := &QueryVMetricsPlugin{}
	resp, err := p.Execute(context.Background(), &mirastack.ExecuteRequest{
		Params: map[string]string{"query": "up"},
	})
	if err != nil {
		t.Fatalf("Execute should not return an error, got: %v", err)
	}
	var result map[string]interface{}
	if unmarshalErr := json.Unmarshal(resp.Output, &result); unmarshalErr != nil {
		t.Fatalf("failed to unmarshal output: %v", unmarshalErr)
	}
	if _, ok := result["error"]; !ok {
		t.Error("expected error key in output for missing action")
	}
}

func TestExecute_UnknownAction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := &QueryVMetricsPlugin{client: NewVMetricsClient(srv.URL)}
	resp, err := p.Execute(context.Background(), &mirastack.ExecuteRequest{
		ActionID: "nonexistent_action",
		Params:   map[string]string{},
	})
	if err != nil {
		t.Fatalf("Execute should not return an error, got: %v", err)
	}
	var result map[string]interface{}
	if unmarshalErr := json.Unmarshal(resp.Output, &result); unmarshalErr != nil {
		t.Fatalf("failed to unmarshal output: %v", unmarshalErr)
	}
	if _, ok := result["error"]; !ok {
		t.Error("expected error key in output for unknown action")
	}
}
