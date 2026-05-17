package cr_github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const base = "https://api.github.com"

type GithubConfig struct {
	HTTP  *http.Client
	Token string
	Base  string
	Org   string
	Repos map[string]string
}

type ActionsRun struct {
	HeadSHA      string    `json:"head_sha"`
	CreatedAt    time.Time `json:"created_at"`
	Conclusion   string    `json:"conclusion"`
	WorkflowName string    `json:"workflow_name"`
	Name         string    `json:"name"`
	HTMLURL      string    `json:"html_url"`
}

type Repo struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type Org struct {
	Login string `json:"login"`
	URL   string `json:"url"`
}

// NOTE(nasr): fields must be exported for json.Decoder to populate them.
type Blob struct {
	FileSHA  string `json:"sha"`
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

// PRResponse carries the fields needed to open a pull request.
type PRResponse struct {
	Owner string
	Repo  string
	Title string
	Body  string
	Head  string
	Base  string
}

// addHeaders attaches the standard GitHub API headers to a request.
func addHeaders(req *http.Request, token string) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

// note(nasr): disabled but kept — could be useful if this package is ever
// extracted into its own service that needs to discover organisations.
func _(ctx context.Context, client *GithubConfig) (map[string]string, error) {
	url := fmt.Sprintf("%s/user/orgs", base)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	addHeaders(req, client.Token)
	resp, err := client.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch orgs: %s", resp.Status)
	}

	var orgs []Org
	if err := json.NewDecoder(resp.Body).Decode(&orgs); err != nil {
		return nil, err
	}

	result := make(map[string]string, len(orgs))
	for _, org := range orgs {
		result[org.Login] = org.URL
	}
	return result, nil
}

// FetchRepos retrieves all repositories for the configured organisation and
// returns a name→URL map. It also populates config.Repos as a side effect.
func FetchRepos(ctx context.Context, config *GithubConfig) (map[string]string, error) {
	u := fmt.Sprintf("%s/orgs/%s/repos?per_page=100", base, config.Org)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	addHeaders(req, config.Token)
	resp, err := config.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch repos: %s", resp.Status)
	}

	var repos []Repo
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return nil, err
	}

	result := make(map[string]string, len(repos))
	for _, repo := range repos {
		result[repo.Name] = repo.URL
	}

	config.Repos = result
	return result, nil
}

func FetchRecentRuns(ctx context.Context, config *GithubConfig, repo string, limit int) ([]ActionsRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	u := fmt.Sprintf("%s/repos/%s/%s/actions/runs?head_branch=main&per_page=%d&status=completed",
		base, config.Org, repo, limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	addHeaders(req, config.Token)

	resp, err := config.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch recent runs: %s", resp.Status)
	}

	var envelope struct {
		WorkflowRuns []ActionsRun `json:"workflow_runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, err
	}

	return envelope.WorkflowRuns, nil
}

// RequestChanges opens a pull request on the given repo.
func RequestChanges(ctx context.Context, config *GithubConfig, pr PRResponse) (map[string]any, error) {
	u := fmt.Sprintf("%s/repos/%s/%s/pulls", base, pr.Owner, pr.Repo)

	body := map[string]string{
		"title": pr.Title,
		"body":  pr.Body,
		"head":  pr.Head,
		"base":  pr.Base,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	addHeaders(req, config.Token)
	resp, err := config.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("request changes: %s", resp.Status)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}

// FetchRecentCommits retrieves the N most recent commits on the default branch.
func FetchRecentCommits(ctx context.Context, config *GithubConfig, repo string, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	u := fmt.Sprintf("%s/repos/%s/%s/commits?per_page=%d", base, config.Org, repo, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	addHeaders(req, config.Token)
	resp, err := config.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch recent commits: %s", resp.Status)
	}

	var commits []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&commits); err != nil {
		return nil, err
	}

	return commits, nil
}

func FetchPRs(ctx context.Context, config *GithubConfig, repo, state string, limit int) ([]map[string]any, error) {
	if state != "open" && state != "closed" && state != "all" {
		state = "open"
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	u := fmt.Sprintf("%s/repos/%s/%s/pulls?state=%s&per_page=%d", base, config.Org, repo, state, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	addHeaders(req, config.Token)
	resp, err := config.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch prs: %s", resp.Status)
	}

	var prs []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&prs); err != nil {
		return nil, err
	}

	return prs, nil
}

func GetBlob(ctx context.Context, config *GithubConfig, owner, repo string, fileSHAs ...string) ([]Blob, error) {
	if len(fileSHAs) == 0 {
		return nil, fmt.Errorf("no file SHAs provided")
	}

	blobs := make([]Blob, 0, len(fileSHAs))
	for _, sha := range fileSHAs {
		u := fmt.Sprintf("%s/repos/%s/%s/git/blobs/%s", base, owner, repo, sha)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}

		addHeaders(req, config.Token)
		resp, err := config.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("get blob %s: %s", sha, resp.Status)
		}

		var blob Blob
		if err := json.NewDecoder(resp.Body).Decode(&blob); err != nil {
			return nil, err
		}

		blobs = append(blobs, blob)
	}

	return blobs, nil
}

func CreateBlob(ctx context.Context, config *GithubConfig, owner, repo, content string) (Blob, error) {
	u := fmt.Sprintf("%s/repos/%s/%s/git/blobs", base, owner, repo)

	body := map[string]string{
		"content": content,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return Blob{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(bodyBytes))
	if err != nil {
		return Blob{}, err
	}

	addHeaders(req, config.Token)
	resp, err := config.HTTP.Do(req)
	if err != nil {
		return Blob{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return Blob{}, fmt.Errorf("create blob: %s", resp.Status)
	}

	var blob Blob
	if err := json.NewDecoder(resp.Body).Decode(&blob); err != nil {
		return Blob{}, err
	}

	return blob, nil
}
