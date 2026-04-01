# MIRASTACK Plugin: Query VMetrics

Go plugin for querying **VictoriaMetrics** (Prometheus-compatible) from MIRASTACK workflows. Part of the core observability plugin suite.

The `v` prefix denotes Victoria-specific. Enterprise versions for other metrics backends follow the same plugin contract: `query-ddmetrics` (Datadog), `query-mmetrics` (Grafana Mimir), etc.

## Capabilities

| Action | Description |
|--------|-------------|
| `instant_query` | Execute MetricsQL/PromQL instant query |
| `range_query` | Execute MetricsQL/PromQL range query with start/end/step |
| `label_names` | List all label names |
| `label_values` | List values for a specific label |
| `series` | Find series matching selectors |
| `metadata` | Get metric metadata |

## Configuration

The engine pushes configuration via `ConfigUpdated()`:

| Key | Description |
|-----|-------------|
| `metrics_url` | VictoriaMetrics base URL (e.g., `http://victoriametrics:8428`) |

## Example Workflow Step

```yaml
- id: check-error-rate
  type: plugin
  plugin: query_vmetrics
  params:
    action: range_query
    query: "rate(http_requests_total{status=~'5..'}[5m])"
    start: "-1h"
    end: "now"
    step: "1m"
```

## Development

```bash
go build -o mirastack-plugin-query-vmetrics .
```

## Requirements

- Go 1.23+
- mirastack-sdk-go

## License

AGPL v3 — see [LICENSE](LICENSE).
