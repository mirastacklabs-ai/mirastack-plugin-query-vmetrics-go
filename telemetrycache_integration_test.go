package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	mirastack "github.com/mirastacklabs-ai/mirastack-agents-sdk-go"
)

func TestActionRangeQuery_SanitizesQueryAndUsesDirectFetchWhenEngineNil(t *testing.T) {
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))
	defer srv.Close()

	p := &QueryVMetricsPlugin{client: NewVMetricsClient(srv.URL)} // engine=nil by default
	_, err := p.actionRangeQuery(context.Background(), map[string]string{
		"query": `up"`,
		"step":  "1m",
	}, &mirastack.TimeRange{StartEpochMs: 1700000000000, EndEpochMs: 1700003600000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedQuery != "up" {
		t.Fatalf("expected sanitized query up, got %q", capturedQuery)
	}
}

func TestActionRangeQuery_AdaptiveDefaultStepForLargeRange(t *testing.T) {
	var capturedStep string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedStep = r.URL.Query().Get("step")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))
	defer srv.Close()

	p := &QueryVMetricsPlugin{client: NewVMetricsClient(srv.URL)}
	_, err := p.actionRangeQuery(context.Background(), map[string]string{
		"query": "up",
	}, &mirastack.TimeRange{StartEpochMs: 1700000000000, EndEpochMs: 1700172800000}) // 48h
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedStep != "5m" {
		t.Fatalf("expected adaptive step 5m for 48h range, got %q", capturedStep)
	}
}

func TestActionRangeQuery_RetriesOnceOn422TooManyPoints(t *testing.T) {
	var (
		mu    sync.Mutex
		steps []string
		calls int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		steps = append(steps, r.URL.Query().Get("step"))
		currentCall := calls
		mu.Unlock()

		if currentCall == 1 {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte("too many points for the given step"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))
	defer srv.Close()

	p := &QueryVMetricsPlugin{client: NewVMetricsClient(srv.URL)}
	_, err := p.actionRangeQuery(context.Background(), map[string]string{
		"query": "up",
		"step":  "30s",
	}, &mirastack.TimeRange{StartEpochMs: 1700000000000, EndEpochMs: 1700003600000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Fatalf("expected exactly 2 backend calls (retry once), got %d", calls)
	}
	if len(steps) != 2 || steps[0] != "30s" || steps[1] != "1m" {
		t.Fatalf("expected retry step progression [30s,1m], got %v", steps)
	}
}
