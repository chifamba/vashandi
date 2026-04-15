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

	method := "GET"
	var bodyReader interface{ Read([]byte) (int, error) }
	if reqOpts != nil {
		if reqOpts.Method != "" {
			method = reqOpts.Method
		}
		if reqOpts.Body != nil {
			bodyReader = reqOpts.Body
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return nil, err
	}

	if reqOpts != nil {
		req.Header = reqOpts.Header.Clone()
		req.Host = reqOpts.Host
		req.ContentLength = reqOpts.ContentLength
		req.GetBody = reqOpts.GetBody
		req.TransferEncoding = append([]string(nil), reqOpts.TransferEncoding...)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not connect to %s — ensure the URL points to a GitHub or GitHub Enterprise instance", parsedURL.Hostname())
	}
	return resp, nil
}
