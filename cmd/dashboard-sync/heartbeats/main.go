package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/joho/godotenv"
)

const (
	KibanaURL    = "http://localhost:5601"
	DashboardID  = "d29a51e1-f209-45ed-9971-5391767b2989"
	DataViewID   = "3f662f48-703e-4d47-a141-a6d88b72f3a3"
	KbnXsrfToken = "true"
)

// --- ELASTICSEARCH ---

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

	if hits, ok := result["hits"].(map[string]any); ok {
		if total, ok := hits["total"].(map[string]any); ok {
			fmt.Printf("[DEBUG] Totaal documenten gevonden in index (die matchen met tijd): %v\n", total["value"])
		} else if totalVal, ok := hits["total"].(float64); ok {
			fmt.Printf("[DEBUG] Totaal documenten gevonden in index: %v\n", totalVal)
		}
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

func newESClient() (*elasticsearch.Client, error) {
	cfg := elasticsearch.Config{
		Addresses: []string{os.Getenv("ELASTICSEARCH_URL")},
		Username:  os.Getenv("CONTROLROOM_ES_USER"),
		Password:  os.Getenv("CONTROLROOM_ES_PASS"),
	}
	return elasticsearch.NewClient(cfg)
}

// --- KIBANA API HELPERS ---

func getDashboardAttributes() (map[string]any, []map[string]any, []map[string]any, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/saved_objects/dashboard/%s", KibanaURL, DashboardID), nil)
	if err != nil {
		return nil, nil, nil, err
	}
	req.Header.Set("kbn-xsrf", KbnXsrfToken)
	if u := os.Getenv("KIBANA_USERNAME"); u != "" {
		req.SetBasicAuth(u, os.Getenv("KIBANA_PASSWORD"))
	}

	client := http.DefaultClient
	res, err := client.Do(req)
	if err != nil {
		return nil, nil, nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, nil, nil, fmt.Errorf("kibana dashboard GET failed: %s", res.Status)
	}

	var dashboard map[string]any
	if err := json.NewDecoder(res.Body).Decode(&dashboard); err != nil {
		return nil, nil, nil, fmt.Errorf("decode kibana dashboard: %w", err)
	}

	attributes, ok := dashboard["attributes"].(map[string]any)
	if !ok {
		return nil, nil, nil, fmt.Errorf("missing attributes")
	}

	panelsJSONStr, ok := attributes["panelsJSON"].(string)
	if !ok {
		return nil, nil, nil, fmt.Errorf("missing panelsJSON")
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

func putDashboardAttributes(attributes map[string]any, references []map[string]any) error {
	updateBody := map[string]any{
		"attributes": attributes,
		"references": references,
	}
	bodyBytes, err := json.Marshal(updateBody)
	if err != nil {
		return fmt.Errorf("marshal dashboard update body: %w", err)
	}

	putReq, err := http.NewRequest("PUT", fmt.Sprintf("%s/api/saved_objects/dashboard/%s", KibanaURL, DashboardID), bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	putReq.Header.Set("kbn-xsrf", KbnXsrfToken)
	putReq.Header.Set("Content-Type", "application/json")
	if u := os.Getenv("KIBANA_USERNAME"); u != "" {
		putReq.SetBasicAuth(u, os.Getenv("KIBANA_PASSWORD"))
	}

	client := http.DefaultClient
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

func getAllPanelTitles() (map[string]string, error) {
	reqURL := fmt.Sprintf("%s/api/saved_objects/_find?type=lens&type=visualization&per_page=1000", KibanaURL)
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("kbn-xsrf", KbnXsrfToken)
	if u := os.Getenv("KIBANA_USERNAME"); u != "" {
		req.SetBasicAuth(u, os.Getenv("KIBANA_PASSWORD"))
	}

	client := http.DefaultClient
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

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

func findSavedObjectByTitle(title string, objectType string) (string, error) {
	q := url.QueryEscape(fmt.Sprintf(`"%s"`, title))
	reqURL := fmt.Sprintf("%s/api/saved_objects/_find?type=%s&search_fields=title&search=%s", KibanaURL, objectType, q)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("kbn-xsrf", KbnXsrfToken)
	if u := os.Getenv("KIBANA_USERNAME"); u != "" {
		req.SetBasicAuth(u, os.Getenv("KIBANA_PASSWORD"))
	}

	client := http.DefaultClient
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	var found map[string]any
	if err := json.NewDecoder(res.Body).Decode(&found); err != nil {
		return "", fmt.Errorf("decode find response: %w", err)
	}

	if so, ok := found["saved_objects"].([]any); ok {
		for _, obj := range so {
			item := obj.(map[string]any)
			if attrs, ok := item["attributes"].(map[string]any); ok {
				if attrs["title"] == title {
					return item["id"].(string), nil
				}
			}
		}
	}
	return "", nil
}

func createOrUpdateSavedObject(objectType string, id string, payload map[string]any) (string, error) {
	method := "POST"
	reqURL := fmt.Sprintf("%s/api/saved_objects/%s", KibanaURL, objectType)

	if id != "" {
		method = "PUT"
		reqURL = fmt.Sprintf("%s/%s", reqURL, id)
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequest(method, reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("kbn-xsrf", KbnXsrfToken)
	req.Header.Set("Content-Type", "application/json")
	if u := os.Getenv("KIBANA_USERNAME"); u != "" {
		req.SetBasicAuth(u, os.Getenv("KIBANA_PASSWORD"))
	}

	client := http.DefaultClient
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("kibana object %s failed: %s", method, res.Status)
	}

	var response map[string]any
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	newID, ok := response["id"].(string)
	if !ok {
		return "", fmt.Errorf("missing id in response")
	}
	return newID, nil
}

// --- PAYLOAD GENERATORS ---
func createTSVBPayload(serviceName string) map[string]interface{} {
	visState := map[string]interface{}{
		"title": "Status - " + serviceName,
		"type":  "metrics",
		"aggs":  []interface{}{},
		"params": map[string]interface{}{
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
			"filter": map[string]interface{}{
				"language": "kuery",
				"query":    fmt.Sprintf(`service_id.keyword: "%s"`, serviceName),
			},
			"series": []map[string]interface{}{
				{
					"id":              "61ca57f1-469d-11e7-af02-69e470af7417",
					"color":           "#68BC00",
					"label":           serviceName,
					"split_mode":      "everything",
					"time_range_mode": "entire_time_range",
					"metrics": []map[string]interface{}{
						{
							"id":   "61ca57f2-469d-11e7-af02-69e470af7417",
							"type": "count",
						},
					},
				},
			},
			"background_color_rules": []map[string]interface{}{
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

	return map[string]interface{}{
		"attributes": map[string]interface{}{
			"title":       "Status - " + serviceName,
			"visState":    string(visStateBytes),
			"description": "",
			"uiStateJSON": "{}",
			"kibanaSavedObjectMeta": map[string]interface{}{
				"searchSourceJSON": `{"query":{"query":"","language":"kuery"},"filter":[]}`,
			},
		},
		"references": []interface{}{},
	}
}

func createLensMetricPayload(serviceName string) map[string]interface{} {
	kqlQuery := fmt.Sprintf(`service_id.keyword: "%s"`, serviceName)
	return map[string]interface{}{
		"attributes": map[string]interface{}{
			"title":             "Last Received - " + serviceName,
			"visualizationType": "lnsMetric",
			"state": map[string]interface{}{
				"datasourceStates": map[string]interface{}{
					"formBased": map[string]interface{}{
						"layers": map[string]interface{}{
							"layer1": map[string]interface{}{
								"columnOrder": []string{"col_timestamp"},
								"columns": map[string]interface{}{
									"col_timestamp": map[string]interface{}{
										"label":         "Last Received",
										"customLabel":   true,
										"dataType":      "date",
										"operationType": "last_value",
										"sourceField":   "timestamp",
										"isBucketed":    false,
										"params": map[string]interface{}{
											"sortField":       "timestamp",
											"showArrayValues": false,
										},
									},
								},
							},
						},
					},
				},
				"visualization": map[string]interface{}{
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
				"query": map[string]interface{}{
					"query":    kqlQuery,
					"language": "kuery",
				},
				"filters":        []interface{}{},
				"adHocDataViews": map[string]interface{}{},
			},
		},
		"references": []map[string]interface{}{
			{
				"name": "indexpattern-datasource-layer-layer1",
				"type": "index-pattern",
				"id":   DataViewID,
			},
		},
	}
}

// --- HOOFDLOGICA ---

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

func getPanelTitle(p map[string]any, refs []map[string]any, panelTitles map[string]string) string {
	pID := resolvePanelLensID(p, refs)
	if title, exists := panelTitles[pID]; exists && title != "" {
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

func syncHeartbeatDashboard() {
	es, err := newESClient()
	if err != nil {
		fmt.Printf("Fout bij ES client: %v\n", err)
		return
	}

	services, err := getActiveServicesLastWeek(es)
	if err != nil {
		fmt.Printf("Fout bij ophalen services: %v\n", err)
		return
	}

	attrs, panels, refs, err := getDashboardAttributes()
	if err != nil {
		fmt.Printf("Fout bij ophalen dashboard: %v\n", err)
		return
	}

	panelTitles, err := getAllPanelTitles()
	if err != nil {
		fmt.Printf("Fout bij ophalen titels: %v\n", err)
		return
	}

	changed := false
	activeTSVBIDs := make(map[string]string)
	activeLensIDs := make(map[string]string)

	for _, serviceName := range services {
		tsvbTitle := "Status - " + serviceName
		tsvbID, _ := findSavedObjectByTitle(tsvbTitle, "visualization")
		newTsvbID, err := createOrUpdateSavedObject("visualization", tsvbID, createTSVBPayload(serviceName))
		if err == nil {
			activeTSVBIDs[serviceName] = newTsvbID
		}

		lensTitle := "Last Received - " + serviceName
		lensID, _ := findSavedObjectByTitle(lensTitle, "lens")
		newLensID, err := createOrUpdateSavedObject("lens", lensID, createLensMetricPayload(serviceName))
		if err == nil {
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
			if strings.HasPrefix(title, "Status - ") {
				svc := strings.TrimPrefix(title, "Status - ")
				if activeServiceNames[svc] {
					dynamicTSVBPanels[svc] = p
				} else {
					fmt.Printf("Verwijderd: inactieve status: %s\n", svc)
					changed = true
				}
			} else if strings.HasPrefix(title, "Last Received - ") {
				svc := strings.TrimPrefix(title, "Last Received - ")
				if activeServiceNames[svc] {
					dynamicLensPanels[svc] = p
				} else {
					fmt.Printf("Verwijderd: inactieve lens: %s\n", svc)
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
				fmt.Printf("Toegevoegd: TSVB voor %s\n", serviceName)
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
				fmt.Printf("Toegevoegd: Lens voor %s\n", serviceName)
			}
		}
	}

	dynamicMaxY := 0
	if len(services) > 0 {
		// Berekent de benodigde Y-ruimte op basis van het aantal services
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
		updatedPanelsBytes, _ := json.Marshal(finalPanels)
		attrs["panelsJSON"] = string(updatedPanelsBytes)
		if err := putDashboardAttributes(attrs, refs); err != nil {
			fmt.Printf("Fout bij wegschrijven dashboard: %v\n", err)
			return
		}
		fmt.Println("Heartbeat dashboard succesvol gesynchroniseerd (Grid layout).")
	} else {
		fmt.Println("Geen wijzigingen in heartbeat dashboard (alles up-to-date).")
	}
}

func main() {
	_ = godotenv.Load(".env", "cmd/dashboard-sync/.env")
	if os.Getenv("KIBANA_USERNAME") == "" {
		fmt.Println("Warning: KIBANA_USERNAME not set.")
	}

	ticker := time.NewTicker(5 * time.Second)
	for range ticker.C {
		syncHeartbeatDashboard()
	}
}
