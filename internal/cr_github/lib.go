package cr_github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	cr_config "integration-project-ehb/controlroom/internal/cr_config"
)

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

// FileChange describes one file that should be written to a branch commit.
type FileChange struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type gitObject struct {
	SHA  string `json:"sha"`
	Type string `json:"type"`
	URL  string `json:"url"`
}

type gitReference struct {
	Ref     string    `json:"ref"`
	Object  gitObject `json:"object"`
	NodeID  string    `json:"node_id,omitempty"`
	URL     string    `json:"url,omitempty"`
	Message string    `json:"message,omitempty"`
}

type gitTreeEntry struct {
	Path    string `json:"path"`
	Mode    string `json:"mode"`
	Type    string `json:"type"`
	SHA     string `json:"sha,omitempty"`
	Content string `json:"content,omitempty"`
}

type gitTree struct {
	SHA string `json:"sha"`
}

type gitCommit struct {
	SHA  string    `json:"sha"`
	Tree gitObject `json:"tree"`
}

type githubStatusError struct {
	operation  string
	statusCode int
	status     string
}

func (e *githubStatusError) Error() string {
	return fmt.Sprintf("%s: %s", e.operation, e.status)
}

func (e *githubStatusError) StatusCode() int {
	return e.statusCode
}

// PRResponse carries the fields needed to open a pull request.
// If Owner is empty, config.Org will be used as the owner.
type PRResponse struct {
	Owner string // optional; defaults to config.Org
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
	url := fmt.Sprintf("%s/user/orgs", cr_config.GithubBaseAPI)
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
	u := fmt.Sprintf("%s/orgs/%s/repos?per_page=100", cr_config.GithubBaseAPI, config.Org)
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
		cr_config.GithubBaseAPI, config.Org, repo, limit)

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

// RequestChanges opens a pull request on the configured org and specified repo.
// If PRResponse.Owner is empty, config.Org is used.
func RequestChanges(ctx context.Context, config *GithubConfig, pr PRResponse) (map[string]any, error) {
	owner := pr.Owner
	if owner == "" {
		owner = config.Org
	}

	u := fmt.Sprintf("%s/repos/%s/%s/pulls", cr_config.GithubBaseAPI, owner, pr.Repo)

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

// GetGitReference retrieves a git reference, such as refs/heads/main.
func GetGitReference(ctx context.Context, config *GithubConfig, repo, ref string) (gitReference, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return gitReference{}, fmt.Errorf("git reference must be non-empty")
	}

	escapedRef := url.PathEscape(ref)
	u := fmt.Sprintf("%s/repos/%s/%s/git/ref/heads/%s", cr_config.GithubBaseAPI, config.Org, repo, escapedRef)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return gitReference{}, err
	}

	addHeaders(req, config.Token)
	resp, err := config.HTTP.Do(req)
	if err != nil {
		return gitReference{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return gitReference{}, &githubStatusError{operation: "get git reference", statusCode: resp.StatusCode, status: resp.Status}
	}

	var result gitReference
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return gitReference{}, err
	}

	return result, nil
}

// GetCommit retrieves a git commit object by SHA.
func GetCommit(ctx context.Context, config *GithubConfig, repo, sha string) (gitCommit, error) {
	sha = strings.TrimSpace(sha)
	if sha == "" {
		return gitCommit{}, fmt.Errorf("commit SHA must be non-empty")
	}

	escapedSHA := url.PathEscape(sha)
	u := fmt.Sprintf("%s/repos/%s/%s/git/commits/%s", cr_config.GithubBaseAPI, config.Org, repo, escapedSHA)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return gitCommit{}, err
	}

	addHeaders(req, config.Token)
	resp, err := config.HTTP.Do(req)
	if err != nil {
		return gitCommit{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return gitCommit{}, &githubStatusError{operation: "get git commit", statusCode: resp.StatusCode, status: resp.Status}
	}

	var result gitCommit
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return gitCommit{}, err
	}

	return result, nil
}

// CreateTree creates a git tree from blob SHAs, based on an existing tree.
func CreateTree(ctx context.Context, config *GithubConfig, repo, baseTreeSHA string, entries []gitTreeEntry) (gitTree, error) {
	if strings.TrimSpace(baseTreeSHA) == "" {
		return gitTree{}, fmt.Errorf("base tree SHA must be non-empty")
	}
	if len(entries) == 0 {
		return gitTree{}, fmt.Errorf("tree entries must be non-empty")
	}

	u := fmt.Sprintf("%s/repos/%s/%s/git/trees", cr_config.GithubBaseAPI, config.Org, repo)
	body := map[string]any{
		"base_tree": baseTreeSHA,
		"tree":      entries,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return gitTree{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(bodyBytes))
	if err != nil {
		return gitTree{}, err
	}

	addHeaders(req, config.Token)
	resp, err := config.HTTP.Do(req)
	if err != nil {
		return gitTree{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return gitTree{}, &githubStatusError{operation: "create git tree", statusCode: resp.StatusCode, status: resp.Status}
	}

	var result gitTree
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return gitTree{}, err
	}

	return result, nil
}

// CreateGitReference creates a new git reference (branch) pointing to a specific commit SHA.
func CreateGitReference(ctx context.Context, config *GithubConfig, repo, branchName, sha string) error {
	branchName = strings.TrimSpace(branchName)
	sha = strings.TrimSpace(sha)
	if branchName == "" {
		return fmt.Errorf("branch name must be non-empty")
	}
	if sha == "" {
		return fmt.Errorf("commit SHA must be non-empty")
	}

	u := fmt.Sprintf("%s/repos/%s/%s/git/refs", cr_config.GithubBaseAPI, config.Org, repo)

	body := map[string]string{
		"ref": "refs/heads/" + branchName,
		"sha": sha,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}

	addHeaders(req, config.Token)
	resp, err := config.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return &githubStatusError{operation: "create git reference", statusCode: resp.StatusCode, status: resp.Status}
	}

	return nil
}

// RequestChangesWithFiles writes file changes to a branch, opens a PR, and returns the PR payload.
func RequestChangesWithFiles(ctx context.Context, config *GithubConfig, pr PRResponse, files []FileChange, commitMessage string) (map[string]any, error) {
	owner := pr.Owner
	if owner == "" {
		owner = config.Org
	}
	if strings.TrimSpace(pr.Repo) == "" {
		return nil, fmt.Errorf("repo must be non-empty")
	}
	if strings.TrimSpace(pr.Head) == "" {
		return nil, fmt.Errorf("head must be non-empty")
	}
	if strings.TrimSpace(pr.Base) == "" {
		return nil, fmt.Errorf("base must be non-empty")
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("files must be non-empty")
	}

	commitMessage = strings.TrimSpace(commitMessage)
	if commitMessage == "" {
		commitMessage = pr.Title
	}

	if _, err := GetGitReference(ctx, config, pr.Repo, pr.Head); err == nil {
		return nil, fmt.Errorf("branch %q already exists", pr.Head)
	} else {
		var statusErr *githubStatusError
		if !errors.As(err, &statusErr) || statusErr.StatusCode() != http.StatusNotFound {
			return nil, err
		}
	}

	baseRef, err := GetGitReference(ctx, config, pr.Repo, pr.Base)
	if err != nil {
		return nil, err
	}

	baseCommit, err := GetCommit(ctx, config, pr.Repo, baseRef.Object.SHA)
	if err != nil {
		return nil, err
	}

	entries := make([]gitTreeEntry, 0, len(files))
	for _, file := range files {
		if strings.TrimSpace(file.Path) == "" {
			return nil, fmt.Errorf("file path must be non-empty")
		}
		blob, err := CreateBlob(ctx, config, pr.Repo, file.Content)
		if err != nil {
			return nil, err
		}
		entries = append(entries, gitTreeEntry{
			Path: file.Path,
			Mode: "100644",
			Type: "blob",
			SHA:  blob.FileSHA,
		})
	}

	tree, err := CreateTree(ctx, config, pr.Repo, baseCommit.Tree.SHA, entries)
	if err != nil {
		return nil, err
	}
	if tree.SHA == baseCommit.Tree.SHA {
		return nil, fmt.Errorf("no file changes detected")
	}

	commit, err := CreateCommit(ctx, config, pr.Repo, tree.SHA, baseRef.Object.SHA, commitMessage)
	if err != nil {
		return nil, err
	}

	commitSHA, _ := commit["sha"].(string)
	if strings.TrimSpace(commitSHA) == "" {
		return nil, fmt.Errorf("create commit response missing sha")
	}

	if err := CreateGitReference(ctx, config, pr.Repo, pr.Head, commitSHA); err != nil {
		return nil, err
	}

	return RequestChanges(ctx, config, PRResponse{
		Owner: owner,
		Repo:  pr.Repo,
		Title: pr.Title,
		Body:  pr.Body,
		Head:  pr.Head,
		Base:  pr.Base,
	})
}

// FetchRecentCommits retrieves the N most recent commits on the default branch
// for the configured org and specified repo.
func FetchRecentCommits(ctx context.Context, config *GithubConfig, repo string, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	u := fmt.Sprintf("%s/repos/%s/%s/commits?per_page=%d", cr_config.GithubBaseAPI, config.Org, repo, limit)
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

// FetchPRs retrieves pull requests for the configured org and specified repo.
func FetchPRs(ctx context.Context, config *GithubConfig, repo, state string, limit int) ([]map[string]any, error) {
	if state != "open" && state != "closed" && state != "all" {
		state = "open"
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	u := fmt.Sprintf("%s/repos/%s/%s/pulls?state=%s&per_page=%d", cr_config.GithubBaseAPI, config.Org, repo, state, limit)
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

// GetBlob retrieves blobs from the configured org and specified repo.
// Fetches one or more blobs by their SHA identifiers.
func GetBlob(ctx context.Context, config *GithubConfig, repo string, fileSHAs ...string) ([]Blob, error) {
	if len(fileSHAs) == 0 {
		return nil, fmt.Errorf("no file SHAs provided")
	}

	blobs := make([]Blob, 0, len(fileSHAs))
	for _, sha := range fileSHAs {
		u := fmt.Sprintf("%s/repos/%s/%s/git/blobs/%s", cr_config.GithubBaseAPI, config.Org, repo, sha)
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

// CreateBlob creates a new blob object in the configured org and specified repo,
// returning its SHA identifier.
func CreateBlob(ctx context.Context, config *GithubConfig, repo, content string) (Blob, error) {
	u := fmt.Sprintf("%s/repos/%s/%s/git/blobs", cr_config.GithubBaseAPI, config.Org, repo)

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

// CreateBranchWithMostRecentCommit creates a new branch from the most recent commit on main,
// optionally with new content. Returns the created blob SHA.
func CreateBranchWithMostRecentCommit(ctx context.Context, config *GithubConfig, repo, branchName, content string) (Blob, error) {
	// Fetch the most recent commit SHA from main
	commits, err := FetchRecentCommits(ctx, config, repo, 1)
	if err != nil {
		return Blob{}, err
	}

	if len(commits) == 0 {
		return Blob{}, fmt.Errorf("no commits found on main branch")
	}

	commit := commits[0]
	// note(nastr): @nasr because you we're confused earlier. its a map so you can just reference it like this
	sha, ok := commit["sha"].(string)
	if !ok {
		return Blob{}, fmt.Errorf("invalid commit SHA format")
	}

	if err := CreateGitReference(ctx, config, repo, branchName, sha); err != nil {
		return Blob{}, err
	}

	var blob Blob
	if content != "" {
		blob, err = CreateBlob(ctx, config, repo, content)
		if err != nil {
			return Blob{}, err
		}
	}

	return blob, nil
}

// CreateCommit creates a new commit with the given tree SHA and message, returning the commit SHA.
// parentSHA is the SHA of the parent commit; treeSHA is the SHA of the tree object.
func CreateCommit(ctx context.Context, config *GithubConfig, repo, treeSHA, parentSHA, message string) (map[string]any, error) {
	u := fmt.Sprintf("%s/repos/%s/%s/git/commits", cr_config.GithubBaseAPI, config.Org, repo)

	body := map[string]any{
		"tree":    treeSHA,
		"parents": []string{parentSHA},
		"message": message,
		"author": map[string]string{
			"name":  cr_config.GithubJarvisName,
			"email": cr_config.GithubJarvisMail,
		},
		"committer": map[string]string{
			"name":  cr_config.GithubJarvisName,
			"email": cr_config.GithubJarvisMail,
		},
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
		return nil, fmt.Errorf("create commit: %s", resp.Status)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}
