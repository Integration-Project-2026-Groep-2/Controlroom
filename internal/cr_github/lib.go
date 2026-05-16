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

// TODO(nasr): i think a response is the correct name for this but we can always refactor this in the future
type PRResponse struct {
	Owner string
	Repo  string
	Title string
	Body  string
	Head  string
	Base  string
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

// note(nasr): disabling this function but also keeping it because it could prove usefull in the future
// we have one organization but if we ever decide to seperate this tool form this service
// it could be interesting to retrieve all of the orginazations a users has or somethign
// and based on that perform the other operations
// just thinking. well now that we are here. doing this not so cool work on mcp servers, ai, rag, etc.
// it did allow me to discover the existing tools and software further. Github is something very widely used.
// not really a big good thing but it does allow you to do some interesting and fun stuff with it.
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

// FetchRepos fills the configuration repos with the fetched repos but also returns the repos so we can use them
// this will probably will piss a lot of people off because it's ugyly but it works so leave me alone
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

	// note(nasr): extra side effect
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
		WorkflowRuns []ActionsRun `json:"workflow_runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, err
	}

	return envelope.WorkflowRuns, nil
}

// NOTE(nasr): all good and well but how does an llm model take a look at the code and take in the contex properly... im confused on that part
// - make a pull request
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

// FetchRecentCommits retrieves recent commits for a repo
func FetchRecentCommits(ctx context.Context, config *GithubConfig, repo string, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	url := fmt.Sprintf("%s/repos/%s/%s/commits?per_page=%d", base, config.Org, repo, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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

type Blob struct {
	owner    string
	repo     string
	file_sha string
	content  string
	encoding string
}

//- file handlers, big large object files thing. pass the hash get the file
//- pass the content make the file
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
			return nil, fmt.Errorf("get blob: %s", resp.Status)
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

