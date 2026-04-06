package main

import (
	"context"
	"fmt"

	"github.com/mirastacklabs-ai/mirastack-sdk-go"
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
		Name:         "query_vmetrics",
		Version:      "0.1.0",
		Description:  "Query VictoriaMetrics for metrics data using MetricsQL (Prometheus-compatible). Supports instant/range queries, label discovery, series matching, and metric metadata.",
		Permissions:  []mirastack.Permission{mirastack.PermissionRead},
		DevOpsStages: []mirastack.DevOpsStage{mirastack.StageObserve},
		Intents: []mirastack.IntentPattern{
			{Pattern: "query metrics", Description: "Query Prometheus/VictoriaMetrics metrics", Priority: 10},
			{Pattern: "check metric", Description: "Check specific metric values", Priority: 8},
			{Pattern: "label values", Description: "List label values", Priority: 5},
		},
		ConfigParams: []mirastack.ConfigParam{
			{Key: "metrics_url", Type: "string", Required: true, Description: "VictoriaMetrics base URL (e.g. http://victoriametrics:8428)"},
		},
	}
}

func (p *QueryVMetricsPlugin) Schema() *mirastack.PluginSchema {
	return &mirastack.PluginSchema{
		InputParams: []mirastack.ParamSchema{
			{Name: "action", Type: "string", Required: true, Description: "One of: instant_query, range_query, label_names, label_values, series, metadata"},
			{Name: "query", Type: "string", Required: false, Description: "PromQL/MetricsQL query expression"},
			{Name: "start", Type: "string", Required: false, Description: "Start time (RFC3339 or relative like -1h)"},
			{Name: "end", Type: "string", Required: false, Description: "End time (RFC3339 or 'now')"},
			{Name: "step", Type: "string", Required: false, Description: "Query resolution step (e.g., 15s, 1m, 5m)"},
			{Name: "label", Type: "string", Required: false, Description: "Label name for label_values action"},
			{Name: "match", Type: "string", Required: false, Description: "Series selector(s) for series action (comma-separated)"},
			{Name: "metric", Type: "string", Required: false, Description: "Metric name for metadata action"},
		},
		OutputParams: []mirastack.ParamSchema{
			{Name: "result", Type: "json", Required: true, Description: "Query result in Prometheus API response format"},
		},
	}
}

func (p *QueryVMetricsPlugin) Execute(ctx context.Context, req *mirastack.ExecuteRequest) (*mirastack.ExecuteResponse, error) {
	if p.logger == nil {
		p.logger, _ = zap.NewProduction()
	}

	action := req.Params["action"]
	if action == "" {
		return &mirastack.ExecuteResponse{
			Output: map[string]string{"error": "action parameter is required"},
			Logs:   []string{"missing required parameter: action"},
		}, nil
	}

	result, err := p.dispatch(ctx, action, req.Params, req.TimeRange)
	if err != nil {
		return &mirastack.ExecuteResponse{
			Output: map[string]string{"error": err.Error()},
			Logs:   []string{fmt.Sprintf("action %s failed: %v", action, err)},
		}, nil
	}

	return &mirastack.ExecuteResponse{
		Output: map[string]string{"result": result},
		Logs:   []string{fmt.Sprintf("action %s completed", action)},
	}, nil
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
	if url, ok := config["metrics_url"]; ok && url != "" {
		p.client = NewVMetricsClient(url)
		if p.logger != nil {
			p.logger.Info("VictoriaMetrics client updated", zap.String("url", url))
		}
	}
}
