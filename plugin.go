package main

import (
	"context"
	"fmt"

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
		Name:         "query_vmetrics",
		Version:      "0.1.0",
		Description:  "Query VictoriaMetrics for metrics data using MetricsQL (Prometheus-compatible). Supports instant/range queries, label discovery, series matching, and metric metadata.",
		Permissions:  []mirastack.Permission{mirastack.PermissionRead},
		DevOpsStages: []mirastack.DevOpsStage{mirastack.StageObserve},
		Actions: []mirastack.Action{
			{
				ID:          "instant_query",
				Description: "Execute an instant PromQL/MetricsQL query at a single point in time",
				Permission:  mirastack.PermissionRead,
				Stages:      []mirastack.DevOpsStage{mirastack.StageObserve},
				InputParams: []mirastack.ParamSchema{
					{Name: "query", Type: "string", Required: true, Description: "PromQL/MetricsQL query expression"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Query result in Prometheus API response format"},
				},
			},
			{
				ID:          "range_query",
				Description: "Execute a range PromQL/MetricsQL query over a time window",
				Permission:  mirastack.PermissionRead,
				Stages:      []mirastack.DevOpsStage{mirastack.StageObserve},
				InputParams: []mirastack.ParamSchema{
					{Name: "query", Type: "string", Required: true, Description: "PromQL/MetricsQL query expression"},
					{Name: "step", Type: "string", Required: false, Description: "Query resolution step (e.g., 15s, 1m, 5m)"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Query result in Prometheus API response format"},
				},
			},
			{
				ID:          "label_names",
				Description: "List all label names in VictoriaMetrics",
				Permission:  mirastack.PermissionRead,
				Stages:      []mirastack.DevOpsStage{mirastack.StageObserve},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Array of label names"},
				},
			},
			{
				ID:          "label_values",
				Description: "List values for a specific label",
				Permission:  mirastack.PermissionRead,
				Stages:      []mirastack.DevOpsStage{mirastack.StageObserve},
				InputParams: []mirastack.ParamSchema{
					{Name: "label", Type: "string", Required: true, Description: "Label name to get values for"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Array of label values"},
				},
			},
			{
				ID:          "series",
				Description: "Find time series matching label selectors",
				Permission:  mirastack.PermissionRead,
				Stages:      []mirastack.DevOpsStage{mirastack.StageObserve},
				InputParams: []mirastack.ParamSchema{
					{Name: "match", Type: "string", Required: true, Description: "Series selector(s) (comma-separated)"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Array of matching series"},
				},
			},
			{
				ID:          "metadata",
				Description: "Get metadata for a metric name",
				Permission:  mirastack.PermissionRead,
				Stages:      []mirastack.DevOpsStage{mirastack.StageObserve},
				InputParams: []mirastack.ParamSchema{
					{Name: "metric", Type: "string", Required: true, Description: "Metric name for metadata lookup"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Metric metadata (type, help, unit)"},
				},
			},
		},
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
	info := p.Info()
	return &mirastack.PluginSchema{
		Actions: info.Actions,
	}
}

func (p *QueryVMetricsPlugin) Execute(ctx context.Context, req *mirastack.ExecuteRequest) (*mirastack.ExecuteResponse, error) {
	if p.logger == nil {
		p.logger, _ = zap.NewProduction()
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

	resp, _ := mirastack.RespondMap(map[string]any{"result": result})
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
