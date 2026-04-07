package main

import (
	"context"
	"fmt"
	"strings"

	mirastack "github.com/mirastacklabs-ai/mirastack-agents-sdk-go"
	"github.com/mirastacklabs-ai/mirastack-agents-sdk-go/datetimeutils"
)

// Action handlers for the query_vmetrics plugin.
// Each action maps to a VictoriaMetrics Prometheus-compatible API endpoint.

func (p *QueryVMetricsPlugin) actionInstantQuery(ctx context.Context, params map[string]string, tr *mirastack.TimeRange) (string, error) {
	query := params["query"]
	if query == "" {
		return "", fmt.Errorf("query parameter is required for instant_query")
	}
	var evalTime *string
	if tr != nil && tr.EndEpochMs > 0 {
		t := datetimeutils.FormatEpochSeconds(tr.EndEpochMs)
		evalTime = &t
	} else if t := params["time"]; t != "" {
		evalTime = &t
	}
	result, err := p.client.InstantQuery(ctx, query, evalTime)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func (p *QueryVMetricsPlugin) actionRangeQuery(ctx context.Context, params map[string]string, tr *mirastack.TimeRange) (string, error) {
	query := params["query"]
	if query == "" {
		return "", fmt.Errorf("query parameter is required for range_query")
	}

	var start, end string
	if tr != nil && tr.StartEpochMs > 0 {
		start = datetimeutils.FormatEpochSeconds(tr.StartEpochMs)
		end = datetimeutils.FormatEpochSeconds(tr.EndEpochMs)
	} else {
		start = params["start"]
		if start == "" {
			start = "-1h"
		}
		end = params["end"]
		if end == "" {
			end = "now"
		}
	}

	step := params["step"]
	if step == "" {
		step = "1m"
	}
	result, err := p.client.RangeQuery(ctx, query, start, end, step)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func (p *QueryVMetricsPlugin) actionLabelNames(ctx context.Context, params map[string]string) (string, error) {
	var match []string
	if m := params["match"]; m != "" {
		for _, s := range strings.Split(m, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				match = append(match, s)
			}
		}
	}
	result, err := p.client.LabelNames(ctx, match)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func (p *QueryVMetricsPlugin) actionLabelValues(ctx context.Context, params map[string]string) (string, error) {
	label := params["label"]
	if label == "" {
		return "", fmt.Errorf("label parameter is required for label_values")
	}
	result, err := p.client.LabelValues(ctx, label)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func (p *QueryVMetricsPlugin) actionSeries(ctx context.Context, params map[string]string, tr *mirastack.TimeRange) (string, error) {
	matchRaw := params["match"]
	if matchRaw == "" {
		return "", fmt.Errorf("match parameter is required for series")
	}
	var matchers []string
	for _, m := range strings.Split(matchRaw, ",") {
		m = strings.TrimSpace(m)
		if m != "" {
			matchers = append(matchers, m)
		}
	}

	var start, end string
	if tr != nil && tr.StartEpochMs > 0 {
		start = datetimeutils.FormatEpochSeconds(tr.StartEpochMs)
		end = datetimeutils.FormatEpochSeconds(tr.EndEpochMs)
	} else {
		start = params["start"]
		end = params["end"]
	}

	result, err := p.client.Series(ctx, matchers, start, end)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func (p *QueryVMetricsPlugin) actionMetadata(ctx context.Context, params map[string]string) (string, error) {
	var metric *string
	if m := params["metric"]; m != "" {
		metric = &m
	}
	result, err := p.client.Metadata(ctx, metric)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func (p *QueryVMetricsPlugin) actionDeleteSeries(ctx context.Context, params map[string]string) (string, error) {
	match := params["match"]
	if match == "" {
		return "", fmt.Errorf("match parameter is required for delete_series")
	}
	if err := p.client.DeleteSeries(ctx, match); err != nil {
		return "", err
	}
	return fmt.Sprintf(`{"status":"success","deleted":"%s"}`, match), nil
}

func (p *QueryVMetricsPlugin) actionSnapshot(ctx context.Context) (string, error) {
	result, err := p.client.Snapshot(ctx)
	if err != nil {
		return "", err
	}
	return string(result), nil
}
