package services

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func isGitHubDotCom(hostname string) bool {
	h := strings.ToLower(hostname)
	return h == "github.com" || h == "www.github.com"
}

func GitHubAPIBase(hostname string) string {
	if isGitHubDotCom(hostname) {
		return "https://api.github.com"
	}
	return fmt.Sprintf("https://%s/api/v3", hostname)
}

func ResolveRawGitHubURL(hostname, owner, repo, ref, filePath string) string {
	p := strings.TrimPrefix(filePath, "/")
	if isGitHubDotCom(hostname) {
		return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", owner, repo, ref, p)
	}
	return fmt.Sprintf("https://%s/raw/%s/%s/%s/%s", hostname, owner, repo, ref, p)
}

func GHFetch(ctx context.Context, rawURL string, reqOpts *http.Request) (*http.Response, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %v", err)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req := reqOpts
	if req == nil {
		req, err = http.NewRequestWithContext(ctx, "GET", rawURL, nil)
		if err != nil {
			return nil, err
		}
	} else {
		req = req.WithContext(ctx)
		// Assume URL is already set properly on the provided request,
		// or at least it matches the provided string.
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not connect to %s — ensure the URL points to a GitHub or GitHub Enterprise instance", parsedURL.Hostname())
	}
	return resp, nil
}
