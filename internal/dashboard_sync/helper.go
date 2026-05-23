package sync

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	config "integration-project-ehb/controlroom/internal/cr_config"
	"net/http"
	"net/url"
)

// note(nasr): i don't know if this will work but kibana requires a http connection while it's behind https
// at the moomenet. maybe we can use a self signed sertificate or something.
// DashboardHttpClient returns an HTTP client configured to handle HTTPS with self-signed certificates.
func DashboardHttpClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
}

func setDashboardBasicAuth(req *http.Request) {
	if u := config.KibanaConfig.DashboardUser; u != "" {
		req.SetBasicAuth(u, config.KibanaConfig.DashboardPassword)
	}
}

// getAllPanelTitles returns the titles for Kibana lens and visualization saved objects.
func getAllPanelTitles() (map[string]string, error) {
	reqURL := fmt.Sprintf("%s/api/saved_objects/_find?type=lens&type=visualization&per_page=1000", config.KibanaConfig.Url)
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("kbn-xsrf", config.KbnXsrfToken)
	setDashboardBasicAuth(req)

	client := DashboardHttpClient()
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

// findSavedObjectByTitle returns the Kibana saved object ID for an exact title match.
func findSavedObjectByTitle(title string, objectType string) (string, error) {
	q := url.QueryEscape(fmt.Sprintf(`"%s"`, title))
	reqURL := fmt.Sprintf("%s/api/saved_objects/_find?type=%s&search_fields=title&search=%s", config.KibanaConfig.Url, objectType, q)

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

// createOrUpdateSavedObject creates or updates a Kibana saved object and returns its ID.
func createOrUpdateSavedObject(objectType string, id string, payload map[string]any) (string, error) {
	method := "POST"
	reqURL := fmt.Sprintf("%s/api/saved_objects/%s", config.KibanaConfig.Url, objectType)

	if id != "" {
		method = "PUT"
		reqURL = fmt.Sprintf("%s/%s", reqURL, id)
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequest(method, reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("kbn-xsrf", config.KbnXsrfToken)
	req.Header.Set("Content-Type", "application/json")
	setDashboardBasicAuth(req)

	client := DashboardHttpClient()
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
