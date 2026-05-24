package sync

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	config "integration-project-ehb/controlroom/internal/cr_config"
	"integration-project-ehb/controlroom/pkg/logger"
	"io"
	"net/http"
	"net/url"
	"time"
)

// HTTPRetryConfig holds retry parameters
type HTTPRetryConfig struct {
	MaxRetries        int           // Default: 3
	InitialBackoff    time.Duration // Default: 100ms
	MaxBackoff        time.Duration // Default: 2s
	BackoffMultiplier float64       // Default: 1.5
}

var defaultRetryConfig = HTTPRetryConfig{
	MaxRetries:        3,
	InitialBackoff:    100 * time.Millisecond,
	MaxBackoff:        2 * time.Second,
	BackoffMultiplier: 1.5,
}

// DashboardHttpClient returns an HTTP client configured for Kibana API calls
// with proper timeout, TLS handling, and authentication.
// NOTE(nasr): Kibana may be behind HTTPS reverse proxy. This client skips
// certificate verification for development; use proper certs in production.
func DashboardHttpClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  false,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
}

// setDashboardBasicAuth sets Basic Authentication headers on the request.
// CRITICAL: This must be called on every request to ensure credentials are sent.
func setDashboardBasicAuth(req *http.Request) error {
	user := config.KibanaConfig.DashboardUser
	pass := config.KibanaConfig.DashboardPassword

	if user == "" || pass == "" {
		// Log warning if credentials are missing
		logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM,
			"dashboard sync: Kibana credentials not configured (DashboardUser/DashboardPassword empty)"))
		return fmt.Errorf("missing Kibana credentials: DashboardUser or DashboardPassword not set")
	}

	req.SetBasicAuth(user, pass)
	return nil
}

// doRequestWithRetry performs an HTTP request with exponential backoff retry logic.
// Retries on transient errors (timeout, 5xx, auth failures that might be temporary).
func doRequestWithRetry(req *http.Request, retryConfig HTTPRetryConfig) (*http.Response, error) {
	client := DashboardHttpClient()
	backoff := retryConfig.InitialBackoff

	for attempt := 0; attempt <= retryConfig.MaxRetries; attempt++ {
		// Ensure credentials are sent on every attempt
		if err := setDashboardBasicAuth(req); err != nil {
			return nil, err
		}

		// Set required Kibana headers
		req.Header.Set("kbn-xsrf", config.KbnXsrfToken)
		req.Header.Set("Content-Type", "application/json")

		logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM,
			fmt.Sprintf("dashboard sync: HTTP %s %s (attempt %d/%d)", req.Method, req.URL.Path, attempt+1, retryConfig.MaxRetries+1)))

		res, err := client.Do(req)
		if err != nil {
			// Transient error: timeout, connection refused, etc.
			if attempt < retryConfig.MaxRetries {
				logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM,
					fmt.Sprintf("dashboard sync: request failed (attempt %d): %v, retrying in %v", attempt+1, err, backoff)))
				time.Sleep(backoff)
				backoff = min(time.Duration(float64(backoff)*retryConfig.BackoffMultiplier), retryConfig.MaxBackoff)
				continue
			}
			return nil, fmt.Errorf("request failed after %d attempts: %w", retryConfig.MaxRetries+1, err)
		}

		// Handle specific status codes
		switch res.StatusCode {
		case http.StatusOK, http.StatusCreated, http.StatusNoContent:
			// Success
			return res, nil

		case http.StatusUnauthorized: // 401
			// Auth failure: log and don't retry
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM,
				fmt.Sprintf("dashboard sync: authentication failed (401): verify credentials. Response: %s", string(body))))
			return nil, fmt.Errorf("authentication failed (401): check DashboardUser/DashboardPassword configuration")

		case http.StatusForbidden: // 403
			// Permission denied: don't retry
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			logger.Log(logger.NewMessage(logger.ERROR, logger.CONTROLROOM,
				fmt.Sprintf("dashboard sync: permission denied (403): %s", string(body))))
			return nil, fmt.Errorf("permission denied (403): check Kibana user permissions")

		case http.StatusNotFound: // 404
			// Resource not found: don't retry
			res.Body.Close()
			return res, nil // Return as-is; caller will handle 404

		case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
			// Server error: retry
			res.Body.Close()
			if attempt < retryConfig.MaxRetries {
				logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM,
					fmt.Sprintf("dashboard sync: server error %d (attempt %d), retrying in %v", res.StatusCode, attempt+1, backoff)))
				time.Sleep(backoff)
				backoff = min(time.Duration(float64(backoff)*retryConfig.BackoffMultiplier), retryConfig.MaxBackoff)
				continue
			}
			return nil, fmt.Errorf("server error %d after %d attempts", res.StatusCode, retryConfig.MaxRetries+1)

		default:
			// Other status: return as-is
			logger.Log(logger.NewMessage(logger.WARN, logger.CONTROLROOM,
				fmt.Sprintf("dashboard sync: unexpected status %d", res.StatusCode)))
			return res, nil
		}
	}

	return nil, fmt.Errorf("request failed after %d attempts", retryConfig.MaxRetries+1)
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

// findSavedObjectByTitle returns the Kibana saved object ID for an exact title match.
// objectType should be "lens", "visualization", etc.
// Returns empty string if not found (not an error).
func findSavedObjectByTitle(title string, objectType string) (string, error) {
	if title == "" {
		return "", fmt.Errorf("title cannot be empty")
	}
	if objectType == "" {
		return "", fmt.Errorf("objectType cannot be empty")
	}

	q := url.QueryEscape(fmt.Sprintf(`"%s"`, title))
	reqURL := fmt.Sprintf("%s/api/saved_objects/_find?type=%s&search_fields=title&search=%s", config.KibanaConfig.Url, objectType, q)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return "", err
	}

	res, err := doRequestWithRetry(req, defaultRetryConfig)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return "", fmt.Errorf("kibana find saved object failed: %s (body: %s)", res.Status, string(body))
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

	res, err := doRequestWithRetry(req, defaultRetryConfig)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)
		return "", fmt.Errorf("kibana object %s failed: %s (body: %s)", method, res.Status, string(body))
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

// getDashboardAttributes fetches the Kibana dashboard attributes, panels, and references.
// NOTE: Uses retry logic to handle transient failures.
func getDashboardAttributesWithRetry(dashboardID string) (map[string]any, []map[string]any, []map[string]any, error) {
	if dashboardID == "" {
		return nil, nil, nil, fmt.Errorf("missing dashboard id")
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/saved_objects/dashboard/%s", config.KibanaConfig.Url, dashboardID), nil)
	if err != nil {
		return nil, nil, nil, err
	}

	res, err := doRequestWithRetry(req, defaultRetryConfig)
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

// putDashboardAttributes saves the dashboard attributes and references back to Kibana.
// NOTE: Uses retry logic to handle transient failures.
func putDashboardAttributesWithRetry(attributes map[string]any, references []map[string]any, dashboardId string) error {
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

	putRes, err := doRequestWithRetry(putReq, defaultRetryConfig)
	if err != nil {
		return err
	}
	defer putRes.Body.Close()

	if putRes.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(putRes.Body)
		return fmt.Errorf("kibana dashboard PUT failed: %s (body: %s)", putRes.Status, string(body))
	}
	return nil
}

// DEPRECATED: Use getDashboardAttributesWithRetry instead
// Kept for backward compatibility
var getDashboardAttributes = getDashboardAttributesWithRetry

// DEPRECATED: Use putDashboardAttributesWithRetry instead
// Kept for backward compatibility
var putDashboardAttributes = putDashboardAttributesWithRetry
