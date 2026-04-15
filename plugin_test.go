package main

import (
	"encoding/json"
	"testing"

	mirastack "github.com/mirastacklabs-ai/mirastack-agents-sdk-go"
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

	if out["result_count"] != "2" {
		t.Errorf("expected result_count=\"2\", got %v", out["result_count"])
	}
}

func TestEnrichMetricsOutput_Truncation(t *testing.T) {
	// Generate a string longer than 32000 chars.
	long := make([]byte, 33000)
	for i := range long {
		long[i] = 'x'
	}
	out := enrichMetricsOutput("range_query", string(long))

	if out["truncated"] != "true" {
		t.Error("expected truncated=\"true\" for oversized result")
	}
	if len(out["result"]) != 32000 {
		t.Errorf("expected truncated result length=32000, got %d", len(out["result"]))
	}
}

func TestEnrichMetricsOutput_LabelNamesArray(t *testing.T) {
	raw := `{"status":"success","data":["__name__","job","instance"]}`
	out := enrichMetricsOutput("label_names", raw)

	if out["result_count"] != "3" {
		t.Errorf("expected result_count=\"3\", got %v", out["result_count"])
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

func TestInfo_DeleteSeriesAction_AdminPermission(t *testing.T) {
	p := &QueryVMetricsPlugin{}
	info := p.Info()

	var found bool
	for _, action := range info.Actions {
		if action.ID == "delete_series" {
			found = true
			if action.Permission != mirastack.PermissionAdmin {
				t.Errorf("delete_series should have ADMIN permission, got %v", action.Permission)
			}
			if len(action.Intents) == 0 {
				t.Error("delete_series should have per-action intents")
			}
			// Verify match param is required
			hasMatch := false
			for _, p := range action.InputParams {
				if p.Name == "match" && p.Required {
					hasMatch = true
				}
			}
			if !hasMatch {
				t.Error("delete_series should have required 'match' input param")
			}
		}
	}
	if !found {
		t.Fatal("delete_series action not found in Info()")
	}
}

func TestInfo_SnapshotAction_ModifyPermission(t *testing.T) {
	p := &QueryVMetricsPlugin{}
	info := p.Info()

	var found bool
	for _, action := range info.Actions {
		if action.ID == "snapshot" {
			found = true
			if action.Permission != mirastack.PermissionModify {
				t.Errorf("snapshot should have MODIFY permission, got %v", action.Permission)
			}
			if len(action.Intents) == 0 {
				t.Error("snapshot should have per-action intents")
			}
		}
	}
	if !found {
		t.Fatal("snapshot action not found in Info()")
	}
}

func TestInfo_PluginPermissionsIncludeAdminModify(t *testing.T) {
	p := &QueryVMetricsPlugin{}
	info := p.Info()

	hasAdmin := false
	hasModify := false
	for _, perm := range info.Permissions {
		if perm == mirastack.PermissionAdmin {
			hasAdmin = true
		}
		if perm == mirastack.PermissionModify {
			hasModify = true
		}
	}
	if !hasAdmin {
		t.Error("plugin permissions should include ADMIN")
	}
	if !hasModify {
		t.Error("plugin permissions should include MODIFY")
	}
}

func TestActionDeleteSeries_RequiresMatch(t *testing.T) {
	p := &QueryVMetricsPlugin{}
	_, err := p.actionDeleteSeries(nil, map[string]string{})
	if err == nil {
		t.Error("expected error when match is empty")
	}
}

func TestActionSnapshot_RequiresClient(t *testing.T) {
	p := &QueryVMetricsPlugin{}
	_, err := p.dispatch(nil, "snapshot", nil, nil)
	if err == nil {
		t.Error("expected error when client is nil")
	}
}
