package main

import (
	"encoding/json"
	"testing"
)

func TestInfo_HasPerActionIntents(t *testing.T) {
	p := &QueryVMetricsPlugin{}
	info := p.Info()

	if info.Version != "0.2.0" {
		t.Errorf("expected version 0.2.0, got %s", info.Version)
	}

	for _, action := range info.Actions {
		if len(action.Intents) == 0 {
			t.Errorf("action %q has no per-action intents", action.ID)
		}
		for _, intent := range action.Intents {
			if intent.Pattern == "" {
				t.Errorf("action %q has intent with empty pattern", action.ID)
			}
			if intent.Priority == 0 {
				t.Errorf("action %q intent %q has zero priority", action.ID, intent.Pattern)
			}
		}
	}
}

func TestInfo_HasPromptTemplates(t *testing.T) {
	p := &QueryVMetricsPlugin{}
	info := p.Info()

	if len(info.PromptTemplates) == 0 {
		t.Fatal("expected at least one PromptTemplate")
	}
	pt := info.PromptTemplates[0]
	if pt.Name != "query_vmetrics_guide" {
		t.Errorf("expected template name query_vmetrics_guide, got %s", pt.Name)
	}
	if pt.Content == "" {
		t.Error("PromptTemplate content is empty")
	}
}

func TestInfo_PluginIntentsExpanded(t *testing.T) {
	p := &QueryVMetricsPlugin{}
	info := p.Info()

	// Was 3, now should be >=5
	if len(info.Intents) < 5 {
		t.Errorf("expected >=5 plugin-level intents, got %d", len(info.Intents))
	}
}

func TestInfo_ActionDescriptionsEnriched(t *testing.T) {
	p := &QueryVMetricsPlugin{}
	info := p.Info()

	for _, action := range info.Actions {
		if len(action.Description) < 50 {
			t.Errorf("action %q description too short (%d chars): %s", action.ID, len(action.Description), action.Description)
		}
	}
}

func TestEnrichMetricsOutput_BasicFields(t *testing.T) {
	out := enrichMetricsOutput("instant_query", `{"status":"success"}`)

	if out["action"] != "instant_query" {
		t.Errorf("expected action=instant_query, got %v", out["action"])
	}
	if out["status"] != "success" {
		t.Errorf("expected status=success, got %v", out["status"])
	}
}

func TestEnrichMetricsOutput_ExtractsResultCount(t *testing.T) {
	raw := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"__name__":"up"},"value":[1,"1"]},{"metric":{"__name__":"down"},"value":[1,"0"]}]}}`
	out := enrichMetricsOutput("instant_query", raw)

	if out["result_count"] != 2 {
		t.Errorf("expected result_count=2, got %v", out["result_count"])
	}
}

func TestEnrichMetricsOutput_Truncation(t *testing.T) {
	// Generate a string longer than 32000 chars.
	long := make([]byte, 33000)
	for i := range long {
		long[i] = 'x'
	}
	out := enrichMetricsOutput("range_query", string(long))

	if out["truncated"] != true {
		t.Error("expected truncated=true for oversized result")
	}
	if len(out["result"].(string)) != 32000 {
		t.Errorf("expected truncated result length=32000, got %d", len(out["result"].(string)))
	}
}

func TestEnrichMetricsOutput_LabelNamesArray(t *testing.T) {
	raw := `{"status":"success","data":["__name__","job","instance"]}`
	out := enrichMetricsOutput("label_names", raw)

	if out["result_count"] != 3 {
		t.Errorf("expected result_count=3, got %v", out["result_count"])
	}
}

func TestEnrichMetricsOutput_InvalidJSON(t *testing.T) {
	out := enrichMetricsOutput("metadata", "not-json")

	if out["action"] != "metadata" {
		t.Errorf("expected action=metadata, got %v", out["action"])
	}
	if out["result"] != "not-json" {
		t.Error("expected raw result to pass through on invalid JSON")
	}
	if _, exists := out["result_count"]; exists {
		t.Error("result_count should not be set for invalid JSON")
	}
}

func TestSchema_MatchesInfo(t *testing.T) {
	p := &QueryVMetricsPlugin{}
	schema := p.Schema()
	info := p.Info()

	if len(schema.Actions) != len(info.Actions) {
		t.Errorf("schema actions (%d) != info actions (%d)", len(schema.Actions), len(info.Actions))
	}
}

func TestEnrichMetricsOutput_JSONMarshalable(t *testing.T) {
	raw := `{"status":"success","data":{"result":[]}}`
	out := enrichMetricsOutput("instant_query", raw)

	_, err := json.Marshal(out)
	if err != nil {
		t.Errorf("enriched output not JSON-serializable: %v", err)
	}
}
