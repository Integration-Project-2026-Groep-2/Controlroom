package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	config "integration-project-ehb/controlroom/internal/cr_config"
	"integration-project-ehb/controlroom/pkg/logger"

	"github.com/elastic/go-elasticsearch/v9"
)

// note(nasr): TSVB stands for Time Series Visual Builder

var LastWeekQuery string = `{
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

// getActiveServicesLastWeek returns the heartbeat service IDs that were active in the last seven days.
func getActiveServicesLastWeek(es *elasticsearch.Client) ([]string, error) {
	res, err := es.Search(
		es.Search.WithIndex("heartbeats*"),
		es.Search.WithBody(bytes.NewReader([]byte(LastWeekQuery))),
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

	// Create or update TSVB and Lens saved objects for active services
	for _, serviceName := range services {
		// Handle TSVB (Status) visualization
		tsvbTitle := "Status - " + serviceName
		tsvbID, _ := findSavedObjectByTitle(tsvbTitle, "visualization")
		newTsvbID, err := createOrUpdateSavedObject("visualization", tsvbID, createTSVBPayload(serviceName))
		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: failed to create or update heartbeat TSVB visualization for %s: %v", serviceName, err)))
		} else {
			activeTSVBIDs[serviceName] = newTsvbID
		}

		// Handle Lens (Last Received) metric
		lensTitle := "Last Received - " + serviceName
		lensID, _ := findSavedObjectByTitle(lensTitle, "lens")
		newLensID, err := createOrUpdateSavedObject("lens", lensID, createLensMetricPayload(serviceName))
		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: failed to create or update heartbeat lens for %s: %v", serviceName, err)))
		} else {
			activeLensIDs[serviceName] = newLensID
		}
	}

	// Categorize existing panels
	var staticPanels []map[string]any
	dynamicTSVBPanels := make(map[string]map[string]any)
	dynamicLensPanels := make(map[string]map[string]any)
	activeServiceNames := make(map[string]bool)

	for _, s := range services {
		activeServiceNames[s] = true
	}

	// Use shared core to classify and apply prune policy for both TSVB and Lens panels
	prefixKinds := map[string]string{"Status - ": "tsvb", "Last Received - ": "lens"}
	staticPanels, dynMap, pruneChanged := ClassifyAndPrunePanels(config.HeartbeatDashboardId, panels, refs, panelTitles, prefixKinds, activeServiceNames)
	if pruneChanged {
		changed = true
	}
	if m, ok := dynMap["tsvb"]; ok {
		dynamicTSVBPanels = m
	} else {
		dynamicTSVBPanels = map[string]map[string]any{}
	}
	if m, ok := dynMap["lens"]; ok {
		dynamicLensPanels = m
	} else {
		dynamicLensPanels = map[string]map[string]any{}
	}

	// Build final panels with proper grid layout
	var finalPanels []map[string]any

	for i, serviceName := range services {
		col := i % 6
		rowGroup := i / 6

		baseX := col * 8
		tsvbY := rowGroup * 10
		lensY := rowGroup*10 + 5

		tsvbID := activeTSVBIDs[serviceName]
		lensID := activeLensIDs[serviceName]

		// Add or update TSVB panel
		if tsvbID != "" {
			tsvbPanel := buildHeartbeatPanel(serviceName, tsvbID, "visualization", "Status - "+serviceName, baseX, tsvbY, 8, 8, dynamicTSVBPanels)
			if tsvbPanel != nil {
				finalPanels = append(finalPanels, tsvbPanel)
				if _, existed := dynamicTSVBPanels[serviceName]; !existed {
					changed = true
					logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: added heartbeat TSVB panel for service %s", serviceName)))
				}
			}
		}

		// Add or update Lens panel
		if lensID != "" {
			lensPanel := buildHeartbeatPanel(serviceName, lensID, "lens", "Last Received - "+serviceName, baseX, lensY, 8, 4, dynamicLensPanels)
			if lensPanel != nil {
				finalPanels = append(finalPanels, lensPanel)
				if _, existed := dynamicLensPanels[serviceName]; !existed {
					changed = true
					logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: added heartbeat lens panel for service %s", serviceName)))
				}
			}
		}
	}

	// Calculate where static panels should start
	dynamicMaxY := 0
	if len(services) > 0 {
		dynamicMaxY = ((len(services)-1)/6 + 1) * 10
	}

	// Adjust static panel positions to avoid overlap
	minStaticY := 9999
	for _, p := range staticPanels {
		if gd, ok := p["gridData"].(map[string]any); ok {
			if y, ok := gd["y"].(float64); ok && int(y) < minStaticY {
				minStaticY = int(y)
			}
		}
	}

	if len(staticPanels) > 0 && minStaticY < dynamicMaxY {
		shiftY := dynamicMaxY - minStaticY
		for _, p := range staticPanels {
			if gd, ok := p["gridData"].(map[string]any); ok {
				if y, ok := gd["y"].(float64); ok {
					gd["y"] = y + float64(shiftY)
					changed = true
				}
			}
		}
	}

	finalPanels = append(finalPanels, staticPanels...)

	// Persist changes if any
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
