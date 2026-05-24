package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	config "integration-project-ehb/controlroom/internal/cr_config"
	"integration-project-ehb/controlroom/pkg/logger"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
)

// getTodayServices returns the log service names that emitted logs today.
func getTodayServices(es *elasticsearch.Client) ([]string, error) {
	query := `{
		"size": 0,
		"query": {
			"range": {
				"timestamp": {
					"gte": "now/d",
					"time_zone": "Europe/Brussels"
				}
			}
		},
		"aggs": {
			"services": {
				"terms": {
					"field": "service.keyword",
					"size": 50
				}
			}
		}
	}`

	res, err := es.Search(
		es.Search.WithIndex("controlroom-logs*"),
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

	return services, nil
}

// dashboardContainsLens reports whether a lens ID already exists in the dashboard payload.
func dashboardContainsLens(panels []map[string]any, references []map[string]any, lensID string) bool {
	for _, p := range panels {
		if id, ok := p["id"].(string); ok && id == lensID {
			return true
		}
	}
	for _, r := range references {
		if id, ok := r["id"].(string); ok && id == lensID {
			return true
		}
	}
	return false
}

// findLensByTitle returns the Kibana lens ID for the exact logs dashboard title.
func findLensByTitle(serviceName string) (string, error) {
	targetTitle := fmt.Sprintf("Logs - %s", serviceName)
	q := url.QueryEscape(fmt.Sprintf(`"%s"`, targetTitle))
	reqURL := fmt.Sprintf("%s/api/saved_objects/_find?type=lens&search_fields=title&search=%s", config.KibanaConfig.Url, q)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("kbn-xsrf", config.KbnXsrfToken)
	if err := setDashboardBasicAuth(req); err != nil {
		return "", err
	}

	client := DashboardHttpClient()
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("kibana find lens failed: %s", res.Status)
	}

	var found map[string]any
	if err := json.NewDecoder(res.Body).Decode(&found); err != nil {
		return "", fmt.Errorf("decode find response: %w", err)
	}

	// STRICT CHECK: Verify the title matches exactly
	if so, ok := found["saved_objects"].([]any); ok {
		for _, obj := range so {
			item := obj.(map[string]any)
			if attrs, ok := item["attributes"].(map[string]any); ok {
				if attrs["title"] == targetTitle {
					if id, ok := item["id"].(string); ok {
						return id, nil
					}
				}
			}
		}
	}
	return "", nil
}

// getAllLensTitles returns a map of Kibana lens IDs to their titles.
func getAllLensTitles() (map[string]string, error) {
	reqURL := fmt.Sprintf("%s/api/saved_objects/_find?type=lens&per_page=1000", config.KibanaConfig.Url)
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("kbn-xsrf", config.KbnXsrfToken)
	if err := setDashboardBasicAuth(req); err != nil {
		return nil, err
	}

	client := DashboardHttpClient()
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kibana get all lenses failed: %s", res.Status)
	}

	var found map[string]any
	if err := json.NewDecoder(res.Body).Decode(&found); err != nil {
		return nil, fmt.Errorf("decode all lenses response: %w", err)
	}

	titles := make(map[string]string)
	if so, ok := found["saved_objects"].([]any); ok {
		for _, obj := range so {
			item := obj.(map[string]any)
			id, idOk := item["id"].(string)
			if attrs, ok := item["attributes"].(map[string]any); ok && idOk {
				if title, ok := attrs["title"].(string); ok {
					titles[id] = title
				}
			}
		}
	}
	return titles, nil
}

// createLensSavedObject creates a new Kibana lens saved object for a service.
func createLensSavedObject(serviceName string) (string, error) {
	payload := createLensPayload(serviceName)
	body := map[string]any{
		"attributes": payload["attributes"],
		"references": payload["references"],
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal lens payload: %w", err)
	}

	req, err := http.NewRequest(
		"POST",
		fmt.Sprintf("%s/api/saved_objects/lens", config.KibanaConfig.Url),
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("kbn-xsrf", config.KbnXsrfToken)
	req.Header.Set("Content-Type", "application/json")
	if err := setDashboardBasicAuth(req); err != nil {
		return "", err
	}

	client := DashboardHttpClient()
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("kibana lens POST failed: %s", res.Status)
	}

	var created map[string]any
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		return "", fmt.Errorf("decode lens creation response: %w", err)
	}

	id, ok := created["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("missing lens id in response")
	}

	return id, nil
}

// updateLensSavedObject updates an existing Kibana lens saved object for a service.
func updateLensSavedObject(id string, serviceName string) (string, error) {
	payload := createLensPayload(serviceName)
	body := map[string]any{
		"attributes": payload["attributes"],
		"references": payload["references"],
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal lens payload: %w", err)
	}

	req, err := http.NewRequest(
		"PUT",
		fmt.Sprintf("%s/api/saved_objects/lens/%s", config.KibanaConfig.Url, id),
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("kbn-xsrf", config.KbnXsrfToken)
	req.Header.Set("Content-Type", "application/json")
	if err := setDashboardBasicAuth(req); err != nil {
		return "", err
	}

	client := DashboardHttpClient()
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("kibana lens PUT failed: %s", res.Status)
	}

	var updated map[string]any
	if err := json.NewDecoder(res.Body).Decode(&updated); err != nil {
		return "", fmt.Errorf("decode lens update response: %w", err)
	}

	updatedID, ok := updated["id"].(string)
	if !ok || updatedID == "" {
		return "", fmt.Errorf("missing lens id in update response")
	}

	return updatedID, nil
}

// createLensPayload builds the Kibana Lens payload for the logs dashboard.
func createLensPayload(serviceName string) map[string]any {
	kqlQuery := fmt.Sprintf(`service.keyword: "%s" and not (data.keyword: "heartbeat" and (level.keyword: "INFO" or level.keyword: "DEBUG"))`, serviceName)
	return map[string]any{
		"type": "lens",
		"attributes": map[string]any{
			"title":             "Logs - " + serviceName,
			"visualizationType": "lnsDatatable",
			"state": map[string]any{
				"datasourceStates": map[string]any{
					"formBased": map[string]any{
						"layers": map[string]any{
							"layer1": map[string]any{
								"columnOrder": []string{"col_timestamp", "col_level", "col_data", "col_count"},
								"columns": map[string]any{
									"col_timestamp": map[string]any{
										"label": "Timestamp", "customLabel": true,
										"dataType": "date", "operationType": "date_histogram",
										"sourceField": "timestamp", "isBucketed": true,
										"params": map[string]any{"interval": "auto", "includeEmptyRows": false},
									},
									"col_level": map[string]any{
										"label": "Severity", "customLabel": true,
										"dataType": "string", "operationType": "terms",
										"sourceField": "level.keyword", "isBucketed": true,
										"params": map[string]any{"size": 6, "orderBy": map[string]any{"type": "column", "columnId": "col_count"}, "orderDirection": "desc"},
									},
									"col_data": map[string]any{
										"label": "Data", "customLabel": true,
										"dataType": "string", "operationType": "terms",
										"sourceField": "data.keyword", "isBucketed": true,
										"params": map[string]any{"size": 100, "orderBy": map[string]any{"type": "column", "columnId": "col_count"}, "orderDirection": "desc"},
									},
									"col_count": map[string]any{
										"label":    "Count of records",
										"dataType": "number", "operationType": "count",
										"isBucketed": false, "sourceField": "___records___",
										"params": map[string]any{"emptyAsNull": true},
									},
								},
							},
						},
					},
				},
				"visualization": map[string]any{
					"layerId":   "layer1",
					"layerType": "data",
					"columns": []map[string]any{
						{"columnId": "col_timestamp"},
						{"columnId": "col_level"},
						{"columnId": "col_data"},
						{"columnId": "col_count", "hidden": true},
					},
				},
				"query": map[string]any{
					"query":    kqlQuery,
					"language": "kuery",
				},
				"filters": []any{},
			},
		},
		"references": []map[string]any{
			{
				"name": "indexpattern-datasource-layer-layer1",
				"type": "index-pattern",
				"id":   config.LogsDataViewId,
			},
		},
	}
}

// SyncLogsDashboard keeps the logs dashboard aligned with today's active services.
func SyncLogsDashboard(es *elasticsearch.Client) {
	services, err := getTodayServices(es)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: failed to load today's log services: %v", err)))
		return
	}

	attrs, panels, refs, err := getDashboardAttributes(config.LogsDashboardId)
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: failed to load logs dashboard attributes: %v", err)))
		return
	}

	lensTitles, err := getAllLensTitles()
	if err != nil {
		logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: failed to load Kibana lens titles for logs dashboard: %v", err)))
		return
	}

	changed := false
	activeLensIDs := make(map[string]string)

	// Create or update lens saved objects for active services
	for _, serviceName := range services {
		lensID, err := findLensByTitle(serviceName)
		if err != nil {
			logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: skipping lens lookup for %s: %v", serviceName, err)))
			continue
		}

		if lensID == "" {
			// Create new lens
			lensID, err = createLensSavedObject(serviceName)
			if err != nil {
				logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: failed to create logs lens for %s: %v", serviceName, err)))
				continue
			}
			logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: created logs lens for %s", serviceName)))
			changed = true
		} else {
			// Update existing lens
			lensID, err = updateLensSavedObject(lensID, serviceName)
			if err != nil {
				logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: failed to update logs lens for %s: %v", serviceName, err)))
				continue
			}
			logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: updated logs lens for %s", serviceName)))
		}

		if lensID != "" {
			activeLensIDs[serviceName] = lensID
		}
	}

	// Categorize panels: keep static panels, drop stale logs panels, track dynamic lens panels
	var staticPanels []map[string]any
	activePanels := make(map[string]map[string]any) // service name -> panel

	activeServiceSet := make(map[string]bool)
	for _, s := range services {
		activeServiceSet[s] = true
	}

	for _, p := range panels {
		pType, _ := p["type"].(string)

		// Only process lens panels; pass through other types
		if pType != "lens" {
			staticPanels = append(staticPanels, p)
			continue
		}

		// Extract panel title
		panelTitle := getPanelTitle(p, refs, lensTitles)
		if panelTitle == "" {
			// Can't resolve title; treat as static
			staticPanels = append(staticPanels, p)
			continue
		}

		// Check if this is a logs panel
		if after, ok := strings.CutPrefix(panelTitle, "Logs - "); ok {
			serviceName := after
			if activeServiceSet[serviceName] {
				// Keep this panel
				activePanels[serviceName] = p
			} else {
				// Drop stale logs panel
				logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: dropping stale logs panel for service %s", serviceName)))
				changed = true
			}
		} else {
			// Not a logs panel; treat as static
			staticPanels = append(staticPanels, p)
		}
	}

	// Calculate grid positioning
	maxY := 0
	for _, p := range staticPanels {
		if gridData, ok := p["gridData"].(map[string]any); ok {
			if yVal, ok := gridData["y"].(float64); ok {
				if hVal, ok := gridData["h"].(float64); ok {
					if int(yVal+hVal) > maxY {
						maxY = int(yVal + hVal)
					}
				}
			}
		}
	}

	// Add or update panels for active services
	for _, serviceName := range services {
		lensID, ok := activeLensIDs[serviceName]
		if !ok || lensID == "" {
			continue
		}

		if existing, ok := activePanels[serviceName]; ok {
			panelUpdated := false
			if existingID, idOK := existing["id"].(string); !idOK || existingID != lensID {
				existing["id"] = lensID
				panelUpdated = true
			}
			if existingType, typeOK := existing["type"].(string); !typeOK || existingType != "lens" {
				existing["type"] = "lens"
				panelUpdated = true
			}

			staticPanels = append(staticPanels, existing)
			if panelUpdated {
				changed = true
				logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: refreshed logs panel reference for service %s", serviceName)))
			}
		} else {
			// Create new panel
			uniqueIndex := fmt.Sprintf("panel_%d", time.Now().UnixNano())
			newPanel := map[string]any{
				"panelIndex": uniqueIndex,
				"embeddableConfig": map[string]any{
					"enhancements":    map[string]any{},
					"hidePanelTitles": false,
				},
				"gridData": map[string]any{"x": 0, "y": maxY, "w": 48, "h": 15, "i": uniqueIndex},
				"version":  1,
				"type":     "lens",
				"id":       lensID,
			}
			staticPanels = append(staticPanels, newPanel)
			changed = true
			maxY += 15
			logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: added logs panel for service %s", serviceName)))
		}
	}

	// Persist changes if any
	if changed {
		updatedPanelsBytes, err := json.Marshal(staticPanels)
		if err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: failed to marshal logs dashboard panels: %v", err)))
			return
		}

		attrs["panelsJSON"] = string(updatedPanelsBytes)
		if err := putDashboardAttributes(attrs, refs, config.LogsDashboardId); err != nil {
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: failed to save logs dashboard updates: %v", err)))
			return
		}

		logger.Log(logger.NewMessage(logger.INFO, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: logs dashboard updated for %d active services", len(services))))
	} else {
		if len(services) > 0 {
			logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, "dashboard sync: logs dashboard already up to date"))
		}
	}
}
