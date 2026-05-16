package cr_github

import (
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
	Org   string
	Repos map[string]string
}

type Run struct {
	HeadSHA    string    `json:"head_sha"`
	CreatedAt  time.Time `json:"created_at"`
	Conclusion string    `json:"conclusion"`
	Name       string    `json:"name"`
	HTMLURL    string    `json:"html_url"`
}

type Repo struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type Org struct {
	Login string `json:"login"`
	URL   string `json:"url"`
}

// note(nasr): disabling this function but also keeping it because it could prove usefull in the future
// we have one organization but if we ever decide to seperate this tool form this service
// it could be interesting to retrieve all of the orginazations a users has or somethign
// and based on that perform the other operations
// just thinking. well now that we are here. doing this not so cool work on mcp servers, ai, rag, etc.
// it did allow me to discover the existing tools and software further. Github is something very widely used.
// not really a big good thing but it does allow you to do some interesting and fun stuff with it.
func _(ctx context.Context, client *GithubConfig) (map[string]string, error) {
	u := fmt.Sprintf("%s/user/orgs", base)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
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

func FetchRepos(ctx context.Context, config *GithubConfig, org string) (map[string]string, error) {
	u := fmt.Sprintf("%s/orgs/%s/repos?per_page=100", base, org)
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
	return result, nil
}

func FetchRecentRuns(ctx context.Context, config *GithubConfig, repo string, limit int) ([]Run, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	u := fmt.Sprintf("%s/repos/%s/%s/actions/runs?head_branch=main&per_page=%d&status=completed",
		base, config.Org, repo, limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
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
		WorkflowRuns []Run `json:"workflow_runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, err
	}

	return envelope.WorkflowRuns, nil
}

// FetchRecentCommits retrieves recent commits for a repo
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

// FetchPRs retrieves pull requests (open or closed)
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

// note(nasr): something we do a lot and like luca once said. only write a function for something
// if it's something you do more than once
func addHeaders(req *http.Request, token string) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}
