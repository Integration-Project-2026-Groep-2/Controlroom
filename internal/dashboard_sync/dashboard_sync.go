package dashboard_sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"integration-project-ehb/controlroom/internal/cr_config"

	"github.com/elastic/go-elasticsearch/v9"
)


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

func getDashboardAttributes() (map[string]any, []map[string]any, []map[string]any, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/saved_objects/dashboard/%s", config.KibanaURL, config.DashboardID), nil)
	if err != nil {
		return nil, nil, nil, err
	}
	req.Header.Set("kbn-xsrf", config.KbnXsrfToken)
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

	putReq, err := http.NewRequest("PUT", fmt.Sprintf("%s/api/saved_objects/dashboard/%s", config.KibanaURL, config.DashboardID), bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	putReq.Header.Set("kbn-xsrf", config.KbnXsrfToken)
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
	reqURL := fmt.Sprintf("%s/api/saved_objects/_find?type=lens&search_fields=title&search=%s", config.KibanaURL, q)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("kbn-xsrf", config.KbnXsrfToken)
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

func getAllLensTitles() (map[string]string, error) {
	reqURL := fmt.Sprintf("%s/api/saved_objects/_find?type=lens&per_page=1000", config.KibanaURL)
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("kbn-xsrf", config.KbnXsrfToken)
	if u := os.Getenv("KIBANA_USERNAME"); u != "" {
		req.SetBasicAuth(u, os.Getenv("KIBANA_PASSWORD"))
	}

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
		fmt.Sprintf("%s/api/saved_objects/lens", config.KibanaURL),
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("kbn-xsrf", config.KbnXsrfToken)
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
		fmt.Sprintf("%s/api/saved_objects/lens/%s", config.KibanaURL, id),
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("kbn-xsrf", config.KbnXsrfToken)
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
				"id":   config.DataViewId,
			},
		},
	}
}

func getPanelTitle(p map[string]any, refs []map[string]any, lensTitles map[string]string) string {
	// 1. Check via de verborgen ID referentie
	pID := resolvePanelLensID(p, refs)
	if title, exists := lensTitles[pID]; exists && title != "" {
		return title
	}
	// 2. Check of de titel direct in het paneel is opgeslagen (Inline)
	if ec, ok := p["embeddableConfig"].(map[string]any); ok {
		if attrs, ok := ec["attributes"].(map[string]any); ok {
			if title, ok := attrs["title"].(string); ok && title != "" {
				return title
			}
		}
	}
	return ""
}

func resolvePanelLensID(p map[string]any, refs []map[string]any) string {
	// 1. Als het script hem net heeft aangemaakt (nog niet gesaved in de UI)
	if id, ok := p["id"].(string); ok && id != "" {
		return id
	}

	// 2. Kibana 8.x logica: De reference heet "<panelIndex>:savedObjectRef"
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

	// 3. Fallback voor oudere Kibana versies (voor de zekerheid)
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

func syncDashboard(es *elasticsearch.Client) {

	services, err := getTodayServices(es)
	if err != nil {
		fmt.Printf("Fout bij ophalen services: %v\n", err)
		return
	}

	// Haal het huidige dashboard op
	attrs, panels, refs, err := getDashboardAttributes()
	if err != nil {
		fmt.Printf("Fout bij ophalen dashboard attributes: %v\n", err)
		return
	}

	// NIEUW: Haal alle titels op van alle Lenses in Kibana
	lensTitles, err := getAllLensTitles()
	if err != nil {
		fmt.Printf("Fout bij ophalen Lens titels: %v\n", err)
		return
	}

	changed := false

	// 1. Verzamel de Lens ID's van ALLE services die VANDAAG actief zijn
	activeLensIDs := make(map[string]string)
	for _, serviceName := range services {
		lensID, err := findLensByTitle(serviceName)
		if err != nil {
			continue
		}
		if lensID == "" {
			lensID, err = createLensSavedObject(serviceName)
			if err == nil {
				fmt.Printf("Aangemaakte Lens %s voor service %s\n", lensID, serviceName)
			}
		} else {
			lensID, err = updateLensSavedObject(lensID, serviceName)
			if err == nil {
				fmt.Printf("Lens %s bijgewerkt voor service %s\n", lensID, serviceName)
			}
		}
		if lensID != "" {
			activeLensIDs[serviceName] = lensID
		}
	}

	// 2. Filter bestaande panelen (De VEILIGE Garbage Collection)
	var keptPanels []map[string]any

	// Maak een lijstje van actieve service namen voor de inline-check
	activeServiceNames := make(map[string]bool)
	for _, s := range services {
		activeServiceNames[s] = true
	}

	for _, p := range panels {
		pType, _ := p["type"].(string)
		if pType == "lens" {
			lensTitle := getPanelTitle(p, refs, lensTitles)

			if after, ok := strings.CutPrefix(lensTitle, "Logs - "); ok {
				// Knip "Logs - " eraf om de pure service naam te krijgen
				serviceName := after

				// Heeft deze service vandaag logs gestuurd?
				if activeServiceNames[serviceName] {
					keptPanels = append(keptPanels, p)
				} else {
					fmt.Printf("Verwijderd: inactief paneel (titel: '%s')\n", lensTitle)
					changed = true
				}
			} else {
				// VASTE VISUALISATIE
				keptPanels = append(keptPanels, p)
			}
		} else {
			keptPanels = append(keptPanels, p)
		}
	}

	// 3. Bereken de nieuwe start-hoogte (maxY)
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

	// 4. Voeg panelen toe met een UNIEKE panelIndex
	for serviceName, lensID := range activeLensIDs {
		// Check of deze lensID al ergens op het dashboard staat
		exists := false
		for _, p := range keptPanels {
			if resolvePanelLensID(p, refs) == lensID {
				exists = true
				break
			}
		}

		if !exists {
			// FIX: Genereer een onmogelijke unieke ID om Kibana reference botsingen te voorkomen!
			uniqueIndex := fmt.Sprintf("panel_%d", time.Now().UnixNano())

			newPanel := map[string]any{
				"panelIndex": uniqueIndex,
				"embeddableConfig": map[string]any{
					"enhancements": map[string]any{"dynamicActions": map[string]any{"events": []any{}}},
				},
				// We geven de unieke index ook mee aan "i" in de gridData
				"gridData": map[string]any{"x": 0, "y": maxY, "w": 48, "h": 15, "i": uniqueIndex},
				"version":  1,
				"type":     "lens",
				"id":       lensID,
			}
			keptPanels = append(keptPanels, newPanel)
			changed = true
			maxY += 15
			fmt.Printf("Toegevoegd: nieuw paneel voor service %s\n", serviceName)
		}
	}

	// 5. Logica als er middernacht is gepasseerd
	if len(services) == 0 && len(panels) > len(keptPanels) {
		fmt.Println("Geen services actief vandaag. Alle tabellen zijn gewist (vaste panelen behouden).")
	}

	// 6. Sla het dashboard op als de 'keptPanels' lijst afwijkt
	if changed {
		updatedPanelsBytes, err := json.Marshal(keptPanels)
		if err != nil {
			fmt.Printf("Fout bij serialiseren panelsJSON: %v\n", err)
			return
		}
		attrs["panelsJSON"] = string(updatedPanelsBytes)

		if err := putDashboardAttributes(attrs, refs); err != nil {
			fmt.Printf("Fout bij wegschrijven dashboard: %v\n", err)
			return
		}
		fmt.Println("Dashboard panels succesvol bijgewerkt (opgeruimd + toegevoegd).")
	} else if len(services) > 0 {
		fmt.Println("Geen wijzigingen in dashboard panels (alles is up-to-date).")
	}

	fmt.Println("Dashboard sync voltooid om:", time.Now().Format("15:04:05"))
}

func InitDashboardSync(client *elasticsearch.Client) {
	if os.Getenv("KIBANA_USERNAME") == "" {
		fmt.Println("Warning: KIBANA_USERNAME not set. Kibana auth will be disabled.")
	}

	ticker := time.NewTicker(5 * time.Second)
	for range ticker.C {
		syncDashboard(client)
	}
}
