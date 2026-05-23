package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	config "integration-project-ehb/controlroom/internal/cr_config"
	"integration-project-ehb/controlroom/pkg/logger"
	"io"
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

// getDashboardAttributes fetches the Kibana dashboard attributes, panels, and references.
func getDashboardAttributes(dashboardID string) (map[string]any, []map[string]any, []map[string]any, error) {
	if dashboardID == "" {
		return nil, nil, nil, fmt.Errorf("missing dashboard id")
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/saved_objects/dashboard/%s", config.KibanaConfig.Url, dashboardID), nil)
	if err != nil {
		return nil, nil, nil, err
	}
	req.Header.Set("kbn-xsrf", config.KbnXsrfToken)
	setDashboardBasicAuth(req)

	client := DashboardHttpClient()
	res, err := client.Do(req)
	if err != nil {
		return nil, nil, nil, err
	}
	defer res.Body.Close()

	bodyBytes, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return nil, nil, nil, fmt.Errorf("kibana dashboard GET failed: %s: %s", res.Status, string(bodyBytes))
	}

	var dashboard map[string]any
	if err := json.Unmarshal(bodyBytes, &dashboard); err != nil {
		return nil, nil, nil, fmt.Errorf("decode kibana dashboard: %w; body: %s", err, string(bodyBytes))
	}

	attributes, ok := dashboard["attributes"].(map[string]any)
	if !ok {
		return nil, nil, nil, fmt.Errorf("missing attributes in dashboard response")
	}

	panelsJSONStr, ok := attributes["panelsJSON"].(string)
	if !ok {
		return nil, nil, nil, fmt.Errorf("missing panelsJSON in dashboard attributes")
	}

	var panels []map[string]any
	if err := json.Unmarshal([]byte(panelsJSONStr), &panels); err != nil {
		return nil, nil, nil, fmt.Errorf("unmarshal panelsJSON: %w", err)
	}

	var references []map[string]any
	if refsRaw, ok := dashboard["references"].([]any); ok {
		for _, rRaw := range refsRaw {
			if rMap, ok := rRaw.(map[string]any); ok {
				references = append(references, rMap)
			}
		}
	}

	return attributes, panels, references, nil
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

// putDashboardAttributes saves the dashboard attributes and references back to Kibana.
func putDashboardAttributes(attributes map[string]any, references []map[string]any, dashboardId string) error {
	updateBody := map[string]any{
		"attributes": attributes,
		"references": references,
	}
	bodyBytes, err := json.Marshal(updateBody)
	if err != nil {
		return fmt.Errorf("marshal dashboard update body: %w", err)
	}

	putReq, err := http.NewRequest("PUT", fmt.Sprintf("%s/api/saved_objects/dashboard/%s", config.KibanaConfig.Url, dashboardId), bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	putReq.Header.Set("kbn-xsrf", config.KbnXsrfToken)
	putReq.Header.Set("Content-Type", "application/json")
	setDashboardBasicAuth(putReq)

	client := DashboardHttpClient()
	putRes, err := client.Do(putReq)
	if err != nil {
		return err
	}
	defer putRes.Body.Close()

	if putRes.StatusCode != http.StatusOK {
		return fmt.Errorf("kibana dashboard PUT failed: %s", putRes.Status)
	}
	return nil
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
	setDashboardBasicAuth(req)

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

	if so, ok := found["saved_objects"].([]any); ok {
		// STRIKTE CHECK: Controleer of de titel EXACT klopt
		for _, obj := range so {
			item := obj.(map[string]any)
			if attrs, ok := item["attributes"].(map[string]any); ok {
				if attrs["title"] == targetTitle {
					return item["id"].(string), nil
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
	setDashboardBasicAuth(req)

	client := http.DefaultClient
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
	setDashboardBasicAuth(req)

	client := http.DefaultClient
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
	setDashboardBasicAuth(req)

	client := http.DefaultClient
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
										// Keep the same params as the working version
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
						// Keep col_count here but mark it hidden — this is the correct way
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
				"id":   config.HeartbeatDataviewId,
			},
		},
	}
}

// getPanelTitle resolves the display title for a panel from Kibana references or inline attributes.
func getPanelTitle(p map[string]any, refs []map[string]any, lensTitles map[string]string) string {
	pID := resolvePanelLensID(p, refs)
	if title, exists := lensTitles[pID]; exists && title != "" {
		return title
	}
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
	for _, serviceName := range services {
		lensID, err := findLensByTitle(serviceName)
		if err != nil {
			logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: skipping lens lookup for %s: %v", serviceName, err)))
			continue
		}

		if lensID == "" {
			lensID, err = createLensSavedObject(serviceName)
			if err != nil {
				logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: failed to create logs lens for %s: %v", serviceName, err)))
				continue
			}
			logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: created logs lens for %s", serviceName)))
		} else {
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

	var keptPanels []map[string]any
	activeServiceNames := make(map[string]bool)
	for _, serviceName := range services {
		activeServiceNames[serviceName] = true
	}

	for _, p := range panels {
		pType, _ := p["type"].(string)
		if pType == "lens" {
			lensTitle := getPanelTitle(p, refs, lensTitles)
			if _, ok := strings.CutPrefix(lensTitle, "Logs - "); ok {
				keptPanels = append(keptPanels, p)
			} else {
				keptPanels = append(keptPanels, p)
			}
		} else {
			keptPanels = append(keptPanels, p)
		}
	}

	maxY := 0
	for _, p := range keptPanels {
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

	for serviceName, lensID := range activeLensIDs {
		uniqueIndex := fmt.Sprintf("panel_%d", time.Now().UnixNano())
		newPanel := map[string]any{
			"panelIndex": uniqueIndex,
			"embeddableConfig": map[string]any{
				"enhancements": map[string]any{"dynamicActions": map[string]any{"events": []any{}}},
			},
			"gridData": map[string]any{"x": 0, "y": maxY, "w": 48, "h": 15, "i": uniqueIndex},
			"version":  1,
			"type":     "lens",
			"id":       lensID,
		}
		keptPanels = append(keptPanels, newPanel)
		changed = true
		maxY += 15
		logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: added logs panel for service %s", serviceName)))
	}

	if len(services) == 0 && len(panels) > len(keptPanels) {
		logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, "dashboard sync: removed stale logs panels after today's service set became empty"))
	}

	if changed {
		updatedPanelsBytes, err := json.Marshal(keptPanels)
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
		return
	}

	if len(services) > 0 {
		logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, "dashboard sync: logs dashboard already up to date"))
	}
}
