package sync

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	config "integration-project-ehb/controlroom/internal/cr_config"
	"net/http"
	"net/url"
	"time"
)

// DashboardHttpClient returns an HTTP client configured to handle HTTPS with self-signed certificates.
// NOTE(nasr): Kibana may be behind HTTPS reverse proxy while expecting HTTP internally.
// This client skips certificate validation for development; consider using proper certs in production.
func DashboardHttpClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Timeout: 30 * time.Second,
	}
}

func setDashboardBasicAuth(req *http.Request) {
	if u := config.KibanaConfig.DashboardUser; u != "" {
		req.SetBasicAuth(u, config.KibanaConfig.DashboardPassword)
	}
}

// getAllPanelTitles returns a map of Kibana lens and visualization saved object IDs to their titles.
// This is used to resolve panel references without making individual API calls.
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

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kibana get all panel titles failed: %s", res.Status)
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

// findSavedObjectByTitle returns the Kibana saved object ID for an exact title match.
// objectType should be "lens", "visualization", etc.
// Returns empty string if not found (not an error).
func findSavedObjectByTitle(title string, objectType string) (string, error) {
	if title == "" {
		return "", fmt.Errorf("title cannot be empty")
	}

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

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("kibana find saved object failed: %s", res.Status)
	}

	var found map[string]any
	if err := json.NewDecoder(res.Body).Decode(&found); err != nil {
		return "", fmt.Errorf("decode find response: %w", err)
	}

	// STRICT CHECK: Only accept exact title matches
	if so, ok := found["saved_objects"].([]any); ok {
		for _, obj := range so {
			item := obj.(map[string]any)
			if attrs, ok := item["attributes"].(map[string]any); ok {
				if objTitle, ok := attrs["title"].(string); ok && objTitle == title {
					if id, idOk := item["id"].(string); idOk {
						return id, nil
					}
				}
			}
		}
	}
	return "", nil
}

// createOrUpdateSavedObject creates or updates a Kibana saved object and returns its ID.
// If id is empty, a new object is created (POST).
// If id is provided, the object is updated (PUT).
// payload should contain "attributes" and "references" at minimum.
func createOrUpdateSavedObject(objectType string, id string, payload map[string]any) (string, error) {
	if objectType == "" {
		return "", fmt.Errorf("objectType cannot be empty")
	}

	method := "POST"
	reqURL := fmt.Sprintf("%s/api/saved_objects/%s", config.KibanaConfig.Url, objectType)

	if id != "" {
		method = "PUT"
		reqURL = fmt.Sprintf("%s/%s", reqURL, id)
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

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
	if !ok || newID == "" {
		return "", fmt.Errorf("missing or empty id in response")
	}
	return newID, nil
}
