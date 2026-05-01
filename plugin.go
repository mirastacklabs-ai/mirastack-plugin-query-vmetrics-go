package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/mirastacklabs-ai/mirastack-agents-sdk-go"
	"go.uber.org/zap"
)

// QueryVMetricsPlugin queries VictoriaMetrics using the Prometheus-compatible API.
// The "v" prefix denotes Victoria-specific. Enterprise versions for other backends
// (Datadog, Mimir, etc.) will follow the same plugin contract with a different prefix.
type QueryVMetricsPlugin struct {
	client *VMetricsClient
	engine *mirastack.EngineContext
	logger *zap.Logger
}

// SetEngineContext injects the engine callback context (pull model config).
func (p *QueryVMetricsPlugin) SetEngineContext(ec *mirastack.EngineContext) {
	p.engine = ec
}

func (p *QueryVMetricsPlugin) Info() *mirastack.PluginInfo {
	return &mirastack.PluginInfo{
		Name:    "query_vmetrics",
		Version: "0.2.0",
		Description: "Query VictoriaMetrics for metrics data using MetricsQL (Prometheus-compatible). " +
			"Use this plugin when you need current or historical metric values, label discovery, " +
			"series matching, or metric metadata. Start with instant_query for spot checks, " +
			"range_query for trend analysis, and label_values/series for exploration.",
		Permissions:  []mirastack.Permission{mirastack.PermissionRead, mirastack.PermissionModify, mirastack.PermissionAdmin},
		DevOpsStages: []mirastack.DevOpsStage{mirastack.StageObserve, mirastack.StageOperate},
		Actions: []mirastack.Action{
			{
				ID: "instant_query",
				Description: "Execute an instant PromQL/MetricsQL query at a single point in time. " +
					"Use this to check the current value of a metric or evaluate a PromQL expression " +
					"at a specific timestamp. Returns vector or scalar results.",
				Permission: mirastack.PermissionRead,
				Stages:     []mirastack.DevOpsStage{mirastack.StageObserve},
				Intents: []mirastack.IntentPattern{
					{Pattern: "run promql", Description: "Execute a PromQL instant query", Priority: 10},
					{Pattern: "instant metric value", Description: "Get the current value of a metric", Priority: 9},
					{Pattern: "current value of", Description: "Check what a metric reads right now", Priority: 8},
					{Pattern: "evaluate expression", Description: "Evaluate a MetricsQL expression", Priority: 7},
				},
				InputParams: []mirastack.ParamSchema{
					{Name: "query", Type: "string", Required: true, Description: "PromQL/MetricsQL query expression (e.g. 'up{job=\"node\"}' or 'rate(http_requests_total[5m])')"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Query result in Prometheus API response format"},
				},
			},
			{
				ID: "range_query",
				Description: "Execute a range PromQL/MetricsQL query over a time window. " +
					"Use this for trend analysis, anomaly detection, or viewing metric behaviour " +
					"over a period. The engine provides start/end times from user context.",
				Permission: mirastack.PermissionRead,
				Stages:     []mirastack.DevOpsStage{mirastack.StageObserve},
				Intents: []mirastack.IntentPattern{
					{Pattern: "metrics over time", Description: "View metric trend over a time range", Priority: 10},
					{Pattern: "range query", Description: "Execute a range PromQL query", Priority: 9},
					{Pattern: "metric trend", Description: "Analyze metric behaviour over time", Priority: 8},
					{Pattern: "time series for", Description: "Get time series data for a metric", Priority: 7},
				},
				InputParams: []mirastack.ParamSchema{
					{Name: "query", Type: "string", Required: true, Description: "PromQL/MetricsQL query expression"},
					{Name: "step", Type: "string", Required: false, Description: "Query resolution step (e.g., 15s, 1m, 5m). Defaults to 1m."},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Query result in Prometheus API response format"},
				},
			},
			{
				ID: "label_names",
				Description: "List all label names available in VictoriaMetrics. " +
					"Use this to discover what dimensions are available for filtering. " +
					"Optionally scope with match[] selectors to narrow results.",
				Permission: mirastack.PermissionRead,
				Stages:     []mirastack.DevOpsStage{mirastack.StageObserve},
				Intents: []mirastack.IntentPattern{
					{Pattern: "list label names", Description: "Get all available metric label names", Priority: 9},
					{Pattern: "what labels exist", Description: "Discover available metric dimensions", Priority: 8},
					{Pattern: "available dimensions", Description: "List filterable label dimensions", Priority: 7},
				},
				InputParams: []mirastack.ParamSchema{
					{Name: "match", Type: "string", Required: false, Description: "Series selector(s) to scope label names (comma-separated)"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Array of label names"},
				},
			},
			{
				ID: "label_values",
				Description: "List all values for a specific label across the metric store. " +
					"Use this to find which services, namespaces, or other dimension values exist. " +
					"Essential before building filtered queries.",
				Permission: mirastack.PermissionRead,
				Stages:     []mirastack.DevOpsStage{mirastack.StageObserve},
				Intents: []mirastack.IntentPattern{
					{Pattern: "label values", Description: "List values for a metric label", Priority: 9},
					{Pattern: "which services", Description: "Find service names from metrics", Priority: 8},
					{Pattern: "values of label", Description: "Enumerate values for a dimension", Priority: 7},
				},
				InputParams: []mirastack.ParamSchema{
					{Name: "label", Type: "string", Required: true, Description: "Label name to get values for (e.g. 'job', 'namespace', 'instance')"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Array of label values"},
				},
			},
			{
				ID: "series",
				Description: "Find time series matching label selectors. " +
					"Returns the full label set for each matching series. Use this to explore " +
					"what is being collected for a job, service, or metric name.",
				Permission: mirastack.PermissionRead,
				Stages:     []mirastack.DevOpsStage{mirastack.StageObserve},
				Intents: []mirastack.IntentPattern{
					{Pattern: "find series", Description: "Find time series matching selectors", Priority: 9},
					{Pattern: "matching series", Description: "List series that match a selector", Priority: 8},
					{Pattern: "what series exist for", Description: "Discover series for a metric or job", Priority: 7},
				},
				InputParams: []mirastack.ParamSchema{
					{Name: "match", Type: "string", Required: true, Description: "Series selector(s) (comma-separated, e.g. '{job=\"node\"}')"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Array of matching series with label sets"},
				},
			},
			{
				ID: "metadata",
				Description: "Get metric metadata including type, help text, and unit. " +
					"Use this to understand what a metric measures before querying it. " +
					"Returns HELP, TYPE, and UNIT annotations from metric exposition.",
				Permission: mirastack.PermissionRead,
				Stages:     []mirastack.DevOpsStage{mirastack.StageObserve},
				Intents: []mirastack.IntentPattern{
					{Pattern: "metric metadata", Description: "Get metric type and help text", Priority: 9},
					{Pattern: "what does metric measure", Description: "Understand a metric's purpose", Priority: 8},
					{Pattern: "describe metric", Description: "Describe a metric name", Priority: 7},
				},
				InputParams: []mirastack.ParamSchema{
					{Name: "metric", Type: "string", Required: true, Description: "Metric name for metadata lookup (e.g. 'node_cpu_seconds_total')"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Metric metadata (type, help, unit)"},
				},
			},
			{
				ID: "delete_series",
				Description: "Delete time series matching a label selector from VictoriaMetrics. " +
					"This is a destructive ADMIN operation that permanently removes matching series. " +
					"Use only for cardinality emergencies or data cleanup. Requires approval.",
				Permission: mirastack.PermissionAdmin,
				Stages:     []mirastack.DevOpsStage{mirastack.StageOperate},
				Intents: []mirastack.IntentPattern{
					{Pattern: "delete series", Description: "Delete time series from TSDB", Priority: 9},
					{Pattern: "remove metrics", Description: "Remove metric series from storage", Priority: 8},
					{Pattern: "cleanup series", Description: "Clean up unwanted metric series", Priority: 7},
				},
				InputParams: []mirastack.ParamSchema{
					{Name: "match", Type: "string", Required: true, Description: "Series selector to delete (e.g. '{__name__=\"defunct_metric\"}')"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Deletion confirmation"},
				},
			},
			{
				ID: "snapshot",
				Description: "Create a TSDB snapshot for backup purposes. " +
					"Creates a consistent point-in-time snapshot of all metric data. " +
					"Returns the snapshot name which can be used for backup/restore.",
				Permission: mirastack.PermissionModify,
				Stages:     []mirastack.DevOpsStage{mirastack.StageOperate},
				Intents: []mirastack.IntentPattern{
					{Pattern: "create snapshot", Description: "Create a TSDB backup snapshot", Priority: 9},
					{Pattern: "backup metrics", Description: "Back up metric data via snapshot", Priority: 8},
					{Pattern: "tsdb snapshot", Description: "Take a VictoriaMetrics snapshot", Priority: 7},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Snapshot creation result with snapshot name"},
				},
			},
		},
		Intents: []mirastack.IntentPattern{
			{Pattern: "query metrics", Description: "Query Prometheus/VictoriaMetrics metrics", Priority: 10},
			{Pattern: "check metric", Description: "Check specific metric values", Priority: 8},
			{Pattern: "promql", Description: "Execute a PromQL expression", Priority: 7},
			{Pattern: "metricsql", Description: "Execute a MetricsQL expression", Priority: 7},
			{Pattern: "cpu usage", Description: "Check CPU utilisation metrics", Priority: 6},
			{Pattern: "memory usage", Description: "Check memory utilisation metrics", Priority: 6},
			{Pattern: "error rate", Description: "Check request error rates", Priority: 6},
			{Pattern: "request latency", Description: "Check request latency percentiles", Priority: 6},
		},
		PromptTemplates: []mirastack.PromptTemplate{
			{
				Name:        "query_vmetrics_guide",
				Description: "Best practices for using VictoriaMetrics metrics query tools",
				Content: `You have access to VictoriaMetrics metrics tools. Follow these guidelines:

1. DISCOVERY FIRST: Before querying, use label_values("job") or label_names() to find available targets.
2. INSTANT vs RANGE: Use instant_query for current state checks. Use range_query for trend analysis.
3. SCOPING: Always scope queries with label matchers like {job="X", namespace="Y"} to reduce cardinality.
4. RATES: For counters, always wrap with rate() or increase(). Raw counter values are rarely useful.
5. STEP SIZE: For range_query, choose step relative to the time window (e.g., 1m for <1h, 5m for <6h, 15m for <24h).
6. METADATA: Use metadata action to understand metric types (counter, gauge, histogram, summary) before querying.
7. SERIES EXPLORATION: Use series action to check what label combinations exist before building complex queries.
8. COMMON PATTERNS:
   - CPU: rate(node_cpu_seconds_total{mode!="idle"}[5m])
   - Memory: node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes
   - Error rate: rate(http_requests_total{code=~"5.."}[5m]) / rate(http_requests_total[5m])
   - Latency: histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))`,
			},
		},
		ConfigParams: []mirastack.ConfigParam{
			{Key: "metrics_url", Type: "string", Required: true, Description: "VictoriaMetrics base URL (e.g. http://victoriametrics:8428)"},
		},
	}
}

func (p *QueryVMetricsPlugin) Schema() *mirastack.PluginSchema {
	info := p.Info()
	return &mirastack.PluginSchema{
		Actions: info.Actions,
	}
}

func (p *QueryVMetricsPlugin) Execute(ctx context.Context, req *mirastack.ExecuteRequest) (*mirastack.ExecuteResponse, error) {
	if p.logger == nil {
		p.logger, _ = zap.NewProduction()
	}

	// Pull config from engine if client is not yet initialized (cached 15s in SDK).
	if p.client == nil && p.engine != nil {
		if config, err := p.engine.GetConfig(ctx); err == nil {
			p.applyConfig(config)
		}
	}

	action := req.ActionID
	if action == "" {
		action = req.Params["action"]
	}
	if action == "" {
		resp, _ := mirastack.RespondError("action parameter is required")
		resp.Logs = []string{"missing required parameter: action"}
		return resp, nil
	}

	result, err := p.dispatch(ctx, action, req.Params, req.TimeRange)
	if err != nil {
		resp, _ := mirastack.RespondError(err.Error())
		resp.Logs = []string{fmt.Sprintf("action %s failed: %v", action, err)}
		return resp, nil
	}

	resp, _ := mirastack.RespondJSON(enrichMetricsOutput(action, result))
	resp.Logs = []string{fmt.Sprintf("action %s completed", action)}
	return resp, nil
}

func (p *QueryVMetricsPlugin) dispatch(ctx context.Context, action string, params map[string]string, tr *mirastack.TimeRange) (string, error) {
	if p.client == nil {
		return "", fmt.Errorf("plugin not configured: metrics_url not set")
	}

	switch action {
	case "instant_query":
		return p.actionInstantQuery(ctx, params, tr)
	case "range_query":
		return p.actionRangeQuery(ctx, params, tr)
	case "label_names":
		return p.actionLabelNames(ctx, params)
	case "label_values":
		return p.actionLabelValues(ctx, params)
	case "series":
		return p.actionSeries(ctx, params, tr)
	case "metadata":
		return p.actionMetadata(ctx, params)
	case "delete_series":
		return p.actionDeleteSeries(ctx, params)
	case "snapshot":
		return p.actionSnapshot(ctx)
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (p *QueryVMetricsPlugin) HealthCheck(ctx context.Context) error {
	// Pull config from engine (cached 15s in SDK)
	if p.engine != nil {
		config, err := p.engine.GetConfig(ctx)
		if err == nil {
			p.applyConfig(config)
		}
	}
	if p.client == nil {
		return fmt.Errorf("not configured")
	}
	_, err := p.client.LabelNames(ctx, nil)
	return err
}

func (p *QueryVMetricsPlugin) ConfigUpdated(_ context.Context, config map[string]string) error {
	p.applyConfig(config)
	return nil
}

func (p *QueryVMetricsPlugin) applyConfig(config map[string]string) {
	url := config["metrics_url"]
	if url == "" {
		url = os.Getenv("MIRASTACK_METRICS_URL")
	}
	if url != "" {
		p.client = NewVMetricsClient(url)
		if p.logger != nil {
			p.logger.Info("VictoriaMetrics client updated", zap.String("url", url))
		}
	}
}

// enrichMetricsOutput wraps raw query results with metadata for LLM consumption.
// The return type is map[string]string to honour the plugin CallPlugin contract:
// the SDK unmarshals plugin responses into map[string]string and panics on
// non-string JSON values. All numeric / boolean metadata is therefore stringified
// here rather than at the SDK boundary.
func enrichMetricsOutput(action, raw string) map[string]string {
	out := map[string]string{
		"action": action,
		"result": raw,
	}

	const maxLen = 32000
	if len(raw) > maxLen {
		out["result"] = raw[:maxLen]
		out["truncated"] = "true"
	}

	// Try to extract result count from the JSON response.
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		if data, ok := parsed["data"]; ok {
			switch d := data.(type) {
			case map[string]any:
				if result, ok := d["result"]; ok {
					if arr, ok := result.([]any); ok {
						out["result_count"] = strconv.Itoa(len(arr))
					}
				}
			case []any:
				out["result_count"] = strconv.Itoa(len(d))
			}
		}
		if status, ok := parsed["status"].(string); ok {
			out["status"] = status
		}
	}

	return out
}
