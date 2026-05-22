package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	config "integration-project-ehb/controlroom/internal/cr_config"
	"integration-project-ehb/controlroom/pkg/logger"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
)

// getActiveServicesLastWeek returns the heartbeat service IDs that were active in the last seven days.
func getActiveServicesLastWeek(es *elasticsearch.Client) ([]string, error) {
	query := `{
		"size": 0,
		"query": {
			"range": {
				"timestamp": {
					"gte": "now-7d/d",
					"time_zone": "Europe/Brussels"
				}
			}
		},
		"aggs": {
			"services": {
				"terms": {
					"field": "service_id.keyword",
					"size": 100
				}
			}
		}
	}`

	res, err := es.Search(
		es.Search.WithIndex("heartbeats*"),
		es.Search.WithBody(bytes.NewReader([]byte(query))),
	)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch search error: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch response error: %s", res.String())
	}

	var result map[string]any
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode es response: %w", err)
	}

	aggregations, _ := result["aggregations"].(map[string]any)
	servicesAgg, _ := aggregations["services"].(map[string]any)
	buckets, _ := servicesAgg["buckets"].([]any)

	services := make([]string, 0, len(buckets))
	for _, b := range buckets {
		bucket := b.(map[string]any)
		if key, ok := bucket["key"].(string); ok {
			services = append(services, key)
		}
	}

	logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: resolved %d active heartbeat services", len(services))))

	return services, nil
}

// syncHeartbeatDashboard keeps the heartbeat dashboard panels aligned with active services.
func SyncHeartbeatDashboard(es *elasticsearch.Client) {

	services, err := getActiveServicesLastWeek(es)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: failed to load active heartbeat services: %v", err)))
		return
	}

	attrs, panels, refs, err := getDashboardAttributes(config.HeartbeatDashboardId)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: failed to load heartbeat dashboard attributes: %v", err)))
		return
	}

	panelTitles, err := getAllPanelTitles()
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: failed to load Kibana panel titles for heartbeat dashboard: %v", err)))
		return
	}

	changed := false
	activeTSVBIDs := make(map[string]string)
	activeLensIDs := make(map[string]string)

	for _, serviceName := range services {
		tsvbTitle := "Status - " + serviceName
		tsvbID, _ := findSavedObjectByTitle(tsvbTitle, "visualization")
		newTsvbID, err := createOrUpdateSavedObject("visualization", tsvbID, createTSVBPayload(serviceName))
		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: failed to create or update heartbeat TSVB visualization for %s: %v", serviceName, err)))
		} else {
			activeTSVBIDs[serviceName] = newTsvbID
		}

		lensTitle := "Last Received - " + serviceName
		lensID, _ := findSavedObjectByTitle(lensTitle, "lens")
		newLensID, err := createOrUpdateSavedObject("lens", lensID, createLensMetricPayload(serviceName))
		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: failed to create or update heartbeat lens for %s: %v", serviceName, err)))
		} else {
			activeLensIDs[serviceName] = newLensID
		}
	}

	var staticPanels []map[string]any
	dynamicTSVBPanels := make(map[string]map[string]any)
	dynamicLensPanels := make(map[string]map[string]any)

	activeServiceNames := make(map[string]bool)
	for _, s := range services {
		activeServiceNames[s] = true
	}

	for _, p := range panels {
		pType, _ := p["type"].(string)
		if pType == "lens" || pType == "visualization" {
			title := getPanelTitle(p, refs, panelTitles)
			if after, ok := strings.CutPrefix(title, "Status - "); ok {
				svc := after
				if activeServiceNames[svc] {
					dynamicTSVBPanels[svc] = p
				} else {
					logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: dropping stale heartbeat TSVB panel for service %s", svc)))
					changed = true
				}
			} else if after, ok := strings.CutPrefix(title, "Last Received - "); ok {
				svc := after
				if activeServiceNames[svc] {
					dynamicLensPanels[svc] = p
				} else {
					logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: dropping stale heartbeat lens panel for service %s", svc)))
					changed = true
				}
			} else {
				staticPanels = append(staticPanels, p)
			}
		} else {
			staticPanels = append(staticPanels, p)
		}
	}

	var finalPanels []map[string]any

	for i, serviceName := range services {
		col := i % 6
		rowGroup := i / 6

		baseX := col * 8
		tsvbY := rowGroup * 10
		lensY := rowGroup*10 + 5

		tsvbID := activeTSVBIDs[serviceName]
		lensID := activeLensIDs[serviceName]

		if tsvbID != "" {
			if existing, ok := dynamicTSVBPanels[serviceName]; ok {
				oldX, oldY := -1, -1
				iVal := ""
				if gd, ok := existing["gridData"].(map[string]any); ok {
					if xf, ok := gd["x"].(float64); ok {
						oldX = int(xf)
					}
					if yf, ok := gd["y"].(float64); ok {
						oldY = int(yf)
					}
					if iStr, ok := gd["i"].(string); ok {
						iVal = iStr
					}
				}

				if oldX != baseX || oldY != tsvbY {
					existing["gridData"] = map[string]any{"x": baseX, "y": tsvbY, "w": 8, "h": 8, "i": iVal}
					changed = true
				}
				finalPanels = append(finalPanels, existing)
			} else {
				idx := fmt.Sprintf("panel_%d_tsvb", time.Now().UnixNano()+int64(i))
				newPanel := map[string]any{
					"panelIndex": idx,
					"embeddableConfig": map[string]any{
						"enhancements": map[string]any{"dynamicActions": map[string]any{"events": []any{}}},
						"title":        "",
					},
					"gridData": map[string]any{"x": baseX, "y": tsvbY, "w": 8, "h": 8, "i": idx},
					"version":  1,
					"type":     "visualization",
					"id":       tsvbID,
				}
				finalPanels = append(finalPanels, newPanel)
				changed = true
				logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: added heartbeat TSVB panel for service %s", serviceName)))
			}
		}

		if lensID != "" {
			if existing, ok := dynamicLensPanels[serviceName]; ok {
				oldX, oldY := -1, -1
				iVal := ""
				if gd, ok := existing["gridData"].(map[string]any); ok {
					if xf, ok := gd["x"].(float64); ok {
						oldX = int(xf)
					}
					if yf, ok := gd["y"].(float64); ok {
						oldY = int(yf)
					}
					if iStr, ok := gd["i"].(string); ok {
						iVal = iStr
					}
				}

				if oldX != baseX || oldY != lensY {
					existing["gridData"] = map[string]any{"x": baseX, "y": lensY, "w": 8, "h": 4, "i": iVal}
					changed = true
				}
				finalPanels = append(finalPanels, existing)
			} else {
				idx := fmt.Sprintf("panel_%d_lens", time.Now().UnixNano()+int64(i))
				newPanel := map[string]any{
					"panelIndex": idx,
					"embeddableConfig": map[string]any{
						"enhancements": map[string]any{"dynamicActions": map[string]any{"events": []any{}}},
						"timeRange":    map[string]any{"from": "now-1w", "to": "now"},
						"title":        "",
					},
					"gridData": map[string]any{"x": baseX, "y": lensY, "w": 8, "h": 4, "i": idx},
					"version":  1,
					"type":     "lens",
					"id":       lensID,
				}
				finalPanels = append(finalPanels, newPanel)
				changed = true
				logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: added heartbeat lens panel for service %s", serviceName)))
			}
		}
	}

	dynamicMaxY := 0
	if len(services) > 0 {
		dynamicMaxY = ((len(services)-1)/6 + 1) * 10
	}

	minStaticY := 9999
	for _, p := range staticPanels {
		if gd, ok := p["gridData"].(map[string]any); ok {
			if y, ok := gd["y"].(float64); ok {
				if int(y) < minStaticY {
					minStaticY = int(y)
				}
			}
		}
	}

	if len(staticPanels) > 0 && minStaticY != dynamicMaxY {
		shiftY := dynamicMaxY - minStaticY
		for _, p := range staticPanels {
			if gd, ok := p["gridData"].(map[string]any); ok {
				if y, ok := gd["y"].(float64); ok {
					gd["y"] = y + float64(shiftY)
					p["gridData"] = gd
				}
			}
		}
		changed = true
	}

	finalPanels = append(finalPanels, staticPanels...)

	if changed {
		updatedPanelsBytes, err := json.Marshal(finalPanels)
		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: failed to marshal heartbeat dashboard panels: %v", err)))
			return
		}
		attrs["panelsJSON"] = string(updatedPanelsBytes)
		if err := putDashboardAttributes(attrs, refs, config.HeartbeatDashboardId); err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: failed to save heartbeat dashboard updates: %v", err)))
			return
		}
		logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: heartbeat dashboard updated for %d active services", len(services))))
	} else {
		logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, "dashboard sync: heartbeat dashboard already up to date"))
	}
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

// createLensMetricPayload builds the Kibana Lens metric payload for a heartbeat service.
func createLensMetricPayload(serviceName string) map[string]any {
	kqlQuery := fmt.Sprintf(`service_id.keyword: "%s"`, serviceName)
	return map[string]any{
		"attributes": map[string]any{
			"title":             "Last Received - " + serviceName,
			"visualizationType": "lnsMetric",
			"state": map[string]any{
				"datasourceStates": map[string]any{
					"formBased": map[string]any{
						"layers": map[string]any{
							"layer1": map[string]any{
								"columnOrder": []string{"col_timestamp"},
								"columns": map[string]any{
									"col_timestamp": map[string]any{
										"label":         "Last Received",
										"customLabel":   true,
										"dataType":      "date",
										"operationType": "last_value",
										"sourceField":   "timestamp",
										"isBucketed":    false,
										"params": map[string]any{
											"sortField":       "timestamp",
											"showArrayValues": false,
										},
									},
								},
							},
						},
					},
				},
				"visualization": map[string]any{
					"layerId":         "layer1",
					"layerType":       "data",
					"metricAccessor":  "col_timestamp",
					"alignment":       "center",
					"fontSize":        "fit",
					"fontWeight":      "regular",
					"metricPosition":  "top",
					"textAlignment":   "center",
					"primaryPosition": "top",
					"titlesTextAlign": "center",
					"titleWeight":     "normal",
					"primaryAlign":    "center",
					"valueFontMode":   "fit",
				},
				"query": map[string]any{
					"query":    kqlQuery,
					"language": "kuery",
				},
				"filters":        []any{},
				"adHocDataViews": map[string]any{},
			},
		},
		"references": []map[string]any{
			{
				"name": "indexpattern-datasource-layer-layer1",
				"type": "index-pattern",
				"id":   config.HeartbeatDataviewId,
			},
		},
	}
}
