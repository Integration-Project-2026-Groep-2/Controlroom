package sync

import (
	"encoding/json"
	"fmt"
	config "integration-project-ehb/controlroom/internal/cr_config"
	"io"
	"net/http"
)

// buildHeartbeatPanel constructs or updates a heartbeat panel with proper grid data.
// existingPanels: map of existing panels by service name (used to preserve grid positioning if unchanged)
func buildHeartbeatPanel(
	serviceName string,
	savedObjectID string,
	panelType string,
	title string,
	x, y, w, h int,
	existingPanels map[string]map[string]any,
) map[string]any {
	if savedObjectID == "" || panelType == "" {
		return nil
	}

	// Use the saved object ID consistently as the panel index
	panelIndex := savedObjectID

	basePanel := map[string]any{
		"panelIndex": panelIndex,
		"embeddableConfig": map[string]any{
			"enhancements": map[string]any{"dynamicActions": map[string]any{"events": []any{}}},
			"title":        "",
		},
		"gridData": map[string]any{
			"x": x,
			"y": y,
			"w": w,
			"h": h,
			"i": panelIndex,
		},
		"version": 1,
		"type":    panelType,
		"id":      savedObjectID,
	}

	// If this panel already exists, preserve its panel index for stability
	if existing, ok := existingPanels[serviceName]; ok {
		if existingIndex, ok := existing["panelIndex"].(string); ok {
			panelIndex = existingIndex
			basePanel["panelIndex"] = existingIndex
			if gd, ok := basePanel["gridData"].(map[string]any); ok {
				gd["i"] = existingIndex
			}
		}

		if existingEC, ok := existing["embeddableConfig"].(map[string]any); ok {
			basePanel["embeddableConfig"] = existingEC
		}
		if existingVersion, ok := existing["version"]; ok {
			basePanel["version"] = existingVersion
		}

		idUnchanged := false
		if existingID, ok := existing["id"].(string); ok {
			idUnchanged = existingID == savedObjectID
		}

		typeUnchanged := false
		if existingType, ok := existing["type"].(string); ok {
			typeUnchanged = existingType == panelType
		}

		layoutUnchanged := false
		if gd, ok := existing["gridData"].(map[string]any); ok {
			if oldX, okX := gd["x"].(float64); okX && int(oldX) == x {
				if oldY, okY := gd["y"].(float64); okY && int(oldY) == y {
					if oldW, okW := gd["w"].(float64); okW && int(oldW) == w {
						if oldH, okH := gd["h"].(float64); okH && int(oldH) == h {
							layoutUnchanged = true
						}
					}
				}
			}
		}

		if idUnchanged && typeUnchanged && layoutUnchanged {
			return existing
		}

		return basePanel
	}

	return basePanel
}

// createTSVBPayload builds the Kibana TSVB payload for a heartbeat service.
func createTSVBPayload(serviceName string) map[string]any {
	visState := map[string]any{
		"title": "Status - " + serviceName,
		"type":  "metrics",
		"aggs":  []any{},
		"params": map[string]any{
			"id":                   "61ca57f0-469d-11e7-af02-69e470af7417",
			"type":                 "metric",
			"time_field":           "timestamp",
			"index_pattern":        "heartbeats",
			"interval":             "auto",
			"ignore_global_filter": 0,
			"time_range_mode":      "entire_time_range",
			"use_kibana_indexes":   true,
			"axis_position":        "left",
			"axis_formatter":       "number",
			"axis_scale":           "normal",
			"show_legend":          1,
			"truncate_legend":      1,
			"max_lines_legend":     1,
			"show_grid":            1,
			"tooltip_mode":         "show_all",
			"drop_last_bucket":     0,
			"isModelInvalid":       false,
			"filter": map[string]any{
				"language": "kuery",
				"query":    fmt.Sprintf(`service_id.keyword: "%s"`, serviceName),
			},
			"series": []map[string]any{
				{
					"id":              "61ca57f1-469d-11e7-af02-69e470af7417",
					"color":           "#68BC00",
					"label":           serviceName,
					"split_mode":      "everything",
					"time_range_mode": "entire_time_range",
					"metrics": []map[string]any{
						{
							"id":   "61ca57f2-469d-11e7-af02-69e470af7417",
							"type": "count",
						},
					},
				},
			},
			"background_color_rules": []map[string]any{
				{
					"id":               "rule_red",
					"op":               "lte",
					"operator":         "lte",
					"value":            40,
					"background_color": "#FF0000",
					"text_color":       "#FFFFFF",
					"color":            "rgba(255,0,0,1)",
				},
				{
					"id":               "rule_orange",
					"op":               "gte",
					"operator":         "gte",
					"value":            41,
					"background_color": "#FFA500",
					"text_color":       "#FFFFFF",
					"color":            "rgba(255,165,0,1)",
				},
				{
					"id":               "rule_green",
					"op":               "gte",
					"operator":         "gte",
					"value":            52,
					"background_color": "#00FF00",
					"text_color":       "#000000",
					"color":            "rgba(0,255,0,1)",
				},
			},
		},
	}
	visStateBytes, _ := json.Marshal(visState)

	return map[string]any{
		"attributes": map[string]any{
			"title":       "Status - " + serviceName,
			"visState":    string(visStateBytes),
			"description": "",
			"uiStateJSON": "{}",
			"kibanaSavedObjectMeta": map[string]any{
				"searchSourceJSON": `{"query":{"query":"","language":"kuery"},"filter":[]}`,
			},
		},
		"references": []any{},
	}
}

// panelExists reports whether a lens panel with the given ID is already present.
func panelExists(panels []map[string]any, id string) bool {
	for _, p := range panels {
		if t, ok := p["type"].(string); ok && t == "lens" {
			if pid, ok := p["id"].(string); ok && pid == id {
				return true
			}
		}
	}
	return false
}

// findPanelIndexByTitle returns the index of the panel with the matching title.
func findPanelIndexByTitle(panels []map[string]any, title string) int {
	for i, p := range panels {
		if attrs, ok := p["attributes"].(map[string]any); ok {
			if t, ok := attrs["title"].(string); ok && t == title {
				return i
			}
		}
	}
	return -1
}

// getPanelTitle resolves the display title for a panel from Kibana references or inline attributes.
// Returns empty string if title cannot be resolved.
func getPanelTitle(p map[string]any, refs []map[string]any, lensTitles map[string]string) string {
	pID := resolvePanelLensID(p, refs)
	if pID != "" {
		if title, exists := lensTitles[pID]; exists && title != "" {
			return title
		}
	}

	// Fallback: check embedded attributes
	if ec, ok := p["embeddableConfig"].(map[string]any); ok {
		if attrs, ok := ec["attributes"].(map[string]any); ok {
			if title, ok := attrs["title"].(string); ok && title != "" {
				return title
			}
		}
	}

	return ""
}

// resolvePanelLensID resolves the saved object ID for a panel using its direct or referenced ID.
func resolvePanelLensID(p map[string]any, refs []map[string]any) string {
	if id, ok := p["id"].(string); ok && id != "" {
		return id
	}
	if panelIndex, ok := p["panelIndex"].(string); ok {
		expectedRefName := panelIndex + ":savedObjectRef"
		for _, r := range refs {
			if rName, ok := r["name"].(string); ok && rName == expectedRefName {
				if id, ok := r["id"].(string); ok {
					return id
				}
			}
		}
	}
	if refName, ok := p["panelRefName"].(string); ok && refName != "" {
		for _, r := range refs {
			if rName, ok := r["name"].(string); ok && rName == refName {
				if id, ok := r["id"].(string); ok {
					return id
				}
			}
		}
	}

	return ""
}

// getAllPanelTitles returns a map of Kibana lens and visualization saved object IDs to their titles.
// This is used to resolve panel references without making individual API calls.
func getAllPanelTitles() (map[string]string, error) {
	reqURL := fmt.Sprintf("%s/api/saved_objects/_find?type=lens&type=visualization&per_page=1000", config.KibanaConfig.Url)
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	res, err := doRequestWithRetry(req, defaultRetryConfig)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("kibana get all panel titles failed: %s (body: %s)", res.Status, string(body))
	}

	var found map[string]any
	if err := json.NewDecoder(res.Body).Decode(&found); err != nil {
		return nil, fmt.Errorf("decode all panel titles response: %w", err)
	}

	titles := make(map[string]string)
	if so, ok := found["saved_objects"].([]any); ok {
		for _, obj := range so {
			item := obj.(map[string]any)
			id, idOk := item["id"].(string)
			if attrs, ok := item["attributes"].(map[string]any); ok && idOk {
				if title, ok := attrs["title"].(string); ok && title != "" {
					titles[id] = title
				}
			}
		}
	}
	return titles, nil
}
