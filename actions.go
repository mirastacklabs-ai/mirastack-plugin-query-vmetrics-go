package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	mirastack "github.com/mirastacklabs-ai/mirastack-agents-sdk-go"
	"github.com/mirastacklabs-ai/mirastack-agents-sdk-go/datetimeutils"
	"github.com/mirastacklabs-ai/mirastack-agents-sdk-go/telemetrycache"
)

// Action handlers for the query_vmetrics plugin.
// Each action maps to a VictoriaMetrics Prometheus-compatible API endpoint.

// isValidVMTimeParam returns true if the value is a non-empty string that
// VictoriaMetrics can parse as a time parameter: a relative duration like
// "-1h", "now", an epoch-seconds number, or an RFC3339 timestamp.
// Bare punctuation such as "-" is rejected.
func isValidVMTimeParam(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" || v == "-" || v == "+" {
		return false
	}
	return true
}

func (p *QueryVMetricsPlugin) actionInstantQuery(ctx context.Context, params map[string]string, tr *mirastack.TimeRange) (string, error) {
	query := telemetrycache.SanitizePromQL(params["query"])
	if query == "" {
		return "", fmt.Errorf("query parameter is required for instant_query")
	}

	evalSec := resolveInstantEvalSec(params["time"], tr)
	dsID := resolveDataSourceID(params)
	result, err := telemetrycache.InstantQueryCached(ctx, p.engine, dsID, query, evalSec, func() ([]byte, error) {
		var evalTime *string
		if evalSec > 0 {
			eval := strconv.FormatInt(evalSec, 10)
			evalTime = &eval
		}
		return p.client.InstantQuery(ctx, query, evalTime)
	})
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func (p *QueryVMetricsPlugin) actionRangeQuery(ctx context.Context, params map[string]string, tr *mirastack.TimeRange) (string, error) {
	query := telemetrycache.SanitizePromQL(params["query"])
	if query == "" {
		return "", fmt.Errorf("query parameter is required for range_query")
	}
	startSec, endSec := resolveRangeBoundsSec(params, tr)
	step := params["step"]
	if step == "" {
		step = telemetrycache.AdaptiveStep(startSec*1000, endSec*1000)
	}
	dsID := resolveDataSourceID(params)
	result, err := telemetrycache.WithStepRetry(startSec, endSec, step, func(stepForRun string) ([]byte, error) {
		return telemetrycache.RangeQueryCached(
			ctx,
			p.engine,
			"metrics",
			dsID,
			query,
			startSec,
			endSec,
			stepForRun,
			func(cStart, cEnd int64, chunkStep string) ([]byte, error) {
				return p.client.RangeQuery(
					ctx,
					query,
					strconv.FormatInt(cStart, 10),
					strconv.FormatInt(cEnd, 10),
					chunkStep,
				)
			},
		)
	})
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func resolveDataSourceID(params map[string]string) string {
	for _, k := range []string{"data_source_id", "datasource_id", "integration_id"} {
		if v := strings.TrimSpace(params[k]); v != "" {
			return v
		}
	}
	return "default"
}

func resolveInstantEvalSec(raw string, tr *mirastack.TimeRange) int64 {
	if tr != nil && tr.EndEpochMs > 0 {
		return tr.EndEpochMs / 1000
	}
	nowSec := time.Now().UTC().Unix()
	if sec, ok := vmTimeParamToSec(raw, nowSec); ok {
		return sec
	}
	return nowSec
}

func resolveRangeBoundsSec(params map[string]string, tr *mirastack.TimeRange) (int64, int64) {
	if tr != nil && tr.StartEpochMs > 0 {
		return tr.StartEpochMs / 1000, tr.EndEpochMs / 1000
	}
	nowSec := time.Now().UTC().Unix()
	startSec, startOK := vmTimeParamToSec(params["start"], nowSec)
	endSec, endOK := vmTimeParamToSec(params["end"], nowSec)

	switch {
	case !startOK && !endOK:
		endSec = nowSec
		startSec = endSec - 3600
	case !startOK && endOK:
		startSec = endSec - 3600
	case startOK && !endOK:
		endSec = nowSec
	}
	if endSec <= startSec {
		endSec = nowSec
		startSec = endSec - 3600
	}
	return startSec, endSec
}

func vmTimeParamToSec(raw string, nowSec int64) (int64, bool) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return 0, false
	}
	if v == "now" {
		return nowSec, true
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		if n > 1_000_000_000_000 {
			return n / 1000, true
		}
		return n, true
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		n := int64(f)
		if n > 1_000_000_000_000 {
			return n / 1000, true
		}
		return n, true
	}
	return 0, false
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
		if !isValidVMTimeParam(start) {
			start = ""
		}
		if !isValidVMTimeParam(end) {
			end = ""
		}
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
