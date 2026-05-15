package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/joho/godotenv"
)

const (
	KibanaURL    = "http://localhost:5601"
	DashboardID  = "7a16d71e-4b38-488f-9e69-7165e1742d27"
	DataViewID   = "9a15b50a-940c-4b0e-927d-9499ca1bf8c0"
	KbnXsrfToken = "true" // Verplicht voor Kibana API
)

func getTodayServices(es *elasticsearch.Client) ([]string, error) {
	query := `{
        "size": 0,
        "query": {
            "range": {
                "timestamp": {
                    "gte": "now/d"
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
		es.Search.WithIndex("controlroom-logs"),
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

	// FIX: Haal ook de references lijst op die Kibana gebruikt na een handmatige save
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

func dashboardContainsLens(panels []map[string]any, references []map[string]any, lensID string) bool {
	// 1. Check in panelsJSON (Het formaat dat ons Go script gebruikt)
	for _, p := range panels {
		if id, ok := p["id"].(string); ok && id == lensID {
			return true
		}
	}
	// 2. Check in references (Het formaat dat Kibana gebruikt ná een handmatige save)
	for _, r := range references {
		if id, ok := r["id"].(string); ok && id == lensID {
			return true
		}
	}
	return false
}

func putDashboardAttributes(attributes map[string]any, references []map[string]any) error {
	// FIX: Geef de references weer netjes terug aan Kibana
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

func findLensByTitle(serviceName string) (string, error) {
	targetTitle := fmt.Sprintf("Logs - %s", serviceName)
	// Zet het in aanhalingstekens voor een exact phrase match
	q := url.QueryEscape(fmt.Sprintf(`"%s"`, targetTitle))
	reqURL := fmt.Sprintf("%s/api/saved_objects/_find?type=lens&search_fields=title&search=%s", KibanaURL, q)

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

func newESClient() (*elasticsearch.Client, error) {
	cfg := elasticsearch.Config{
		Addresses: []string{os.Getenv("ELASTICSEARCH_URL")},
		Username:  os.Getenv("CONTROLROOM_ES_USER"),
		Password:  os.Getenv("CONTROLROOM_ES_PASS"),
	}
	return elasticsearch.NewClient(cfg)
}

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
		fmt.Sprintf("%s/api/saved_objects/lens", KibanaURL),
		bytes.NewReader(bodyBytes),
	)
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
		fmt.Sprintf("%s/api/saved_objects/lens/%s", KibanaURL, id),
		bytes.NewReader(bodyBytes),
	)
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

// De structuur voor een Lens Visualisatie (vereenvoudigd)
func createLensPayload(serviceName string) map[string]interface{} {
	// De exacte KQL filter string die Kibana native begrijpt
	kqlQuery := fmt.Sprintf(`service.keyword: "%s" and not (data.keyword: "heartbeat" and (level.keyword: "INFO" or level.keyword: "DEBUG"))`, serviceName)

	return map[string]interface{}{
		"type": "lens",
		"attributes": map[string]interface{}{
			"title":             "Logs - " + serviceName,
			"visualizationType": "lnsDatatable",
			"state": map[string]interface{}{
				"datasourceStates": map[string]interface{}{
					"formBased": map[string]interface{}{
						"layers": map[string]interface{}{
							"layer1": map[string]interface{}{
								"columnOrder": []string{"col_timestamp", "col_level", "col_service", "col_data", "col_count"},
								"columns": map[string]interface{}{
									"col_timestamp": map[string]interface{}{"label": "timestamp", "dataType": "date", "operationType": "date_histogram", "sourceField": "timestamp", "isBucketed": true, "params": map[string]interface{}{"interval": "auto", "includeEmptyRows": false}},
									"col_level":     map[string]interface{}{"label": "Severity", "dataType": "string", "operationType": "terms", "sourceField": "level.keyword", "isBucketed": true, "params": map[string]interface{}{"size": 6, "orderBy": map[string]interface{}{"type": "column", "columnId": "col_count"}, "orderDirection": "desc"}},
									"col_service":   map[string]interface{}{"label": "Service", "dataType": "string", "operationType": "terms", "sourceField": "service.keyword", "isBucketed": true, "params": map[string]interface{}{"size": 100, "orderBy": map[string]interface{}{"type": "column", "columnId": "col_count"}, "orderDirection": "desc"}},
									"col_data":      map[string]interface{}{"label": "Data", "dataType": "string", "operationType": "terms", "sourceField": "data.keyword", "isBucketed": true, "params": map[string]interface{}{"size": 100, "orderBy": map[string]interface{}{"type": "column", "columnId": "col_count"}, "orderDirection": "desc"}},
									"col_count":     map[string]interface{}{"label": "Count of records", "dataType": "number", "operationType": "count", "isBucketed": false, "sourceField": "___records___", "params": map[string]interface{}{"hidden": true}},
								},
							},
						},
					},
				},
				"visualization": map[string]interface{}{
					"layerId":   "layer1",
					"layerType": "data",
					"columns": []map[string]interface{}{
						{"columnId": "col_timestamp"},
						{"columnId": "col_level"},
						{"columnId": "col_service"},
						{"columnId": "col_data"},
						{"columnId": "col_count"},
					},
				},
				// HIER ZIT DE FIX: Geen complexe arrays in filters[], maar gewoon de native Kibana zoekbalk (kuery) invullen
				"query": map[string]interface{}{
					"query":    kqlQuery,
					"language": "kuery",
				},
				"filters": []interface{}{},
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

func syncDashboard() {
	es, err := newESClient()
	if err != nil {
		fmt.Printf("Fout bij maken van Elasticsearch client: %v\n", err)
		return
	}

	services, err := getTodayServices(es)
	if err != nil {
		fmt.Printf("Fout bij ophalen services: %v\n", err)
		return
	}

	if len(services) == 0 {
		fmt.Println("Geen service-waarden gevonden voor vandaag.")
		return
	}

	// fetch current dashboard once (haalt nu ook de refs op)
	attrs, panels, refs, err := getDashboardAttributes()
	if err != nil {
		fmt.Printf("Fout bij ophalen dashboard attributes: %v\n", err)
		return
	}

	changed := false

	// Zoek de onderste rand van het dashboard (maxY)
	maxY := 0
	for _, p := range panels {
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

	for _, serviceName := range services {
		// try to find existing Lens saved object by title
		lensID, err := findLensByTitle(serviceName)
		if err != nil {
			fmt.Printf("Fout bij zoeken naar bestaande Lens voor %s: %v\n", serviceName, err)
			continue
		}

		if lensID == "" {
			// create new Lens
			lensID, err = createLensSavedObject(serviceName)
			if err != nil {
				fmt.Printf("Fout bij aanmaken van Lens voor %s: %v\n", serviceName, err)
				continue
			}
			fmt.Printf("Aangemaakte Lens %s voor service %s\n", lensID, serviceName)
		} else {
			lensID, err = updateLensSavedObject(lensID, serviceName)
			if err != nil {
				fmt.Printf("Fout bij updaten van bestaande Lens voor %s: %v\n", serviceName, err)
				continue
			}
			fmt.Printf("Lens %s bijgewerkt voor service %s\n", lensID, serviceName)
		}

		// HIER IS DE FIX: Enkel nog de slimme check gebruiken!
		if !dashboardContainsLens(panels, refs, lensID) {
			newPanel := map[string]any{
				"panelIndex": fmt.Sprintf("%d", len(panels)+1),
				"embeddableConfig": map[string]any{
					"enhancements": map[string]any{"dynamicActions": map[string]any{"events": []any{}}},
				},
				// Plaats op volle breedte op de Y-coördinaat 'maxY'
				"gridData": map[string]any{"x": 0, "y": maxY, "w": 48, "h": 15},
				"version":  1,
				"type":     "lens",
				"id":       lensID,
			}
			panels = append(panels, newPanel)
			changed = true
			maxY += 15 // Duw de maxY naar beneden voor een eventuele volgende service
			fmt.Printf("Panel toegevoegd voor service %s (lens %s)\n", serviceName, lensID)
		} else {
			fmt.Printf("Panel staat al op het dashboard voor service %s\n", serviceName)
		}
	}

	if changed {
		updatedPanelsBytes, err := json.Marshal(panels)
		if err != nil {
			fmt.Printf("Fout bij serialiseren panelsJSON: %v\n", err)
			return
		}
		attrs["panelsJSON"] = string(updatedPanelsBytes)

		// HIER IS DE FIX: refs netjes meegeven aan de put functie
		if err := putDashboardAttributes(attrs, refs); err != nil {
			fmt.Printf("Fout bij wegschrijven dashboard: %v\n", err)
			return
		}
		fmt.Println("Dashboard panels bijgewerkt.")
	} else {
		fmt.Println("Geen wijzigingen in dashboard panels.")
	}

	fmt.Println("Dashboard sync voltooid om:", time.Now().Format("15:04:05"))
}

func main() {
	// load .env for local development (optional)
	_ = godotenv.Load(".env", "cmd/dashboard-sync/.env")

	if os.Getenv("KIBANA_USERNAME") == "" {
		fmt.Println("Warning: KIBANA_USERNAME not set. Kibana auth will be disabled.")
	}

	ticker := time.NewTicker(5 * time.Second)
	for range ticker.C {
		syncDashboard()
	}
}
