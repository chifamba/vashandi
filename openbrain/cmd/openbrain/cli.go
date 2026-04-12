package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chifamba/vashandi/openbrain/internal/brain"
	"github.com/spf13/cobra"
)

var cliCmd = NewRootCommand(nil)

type cliClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewRootCommand(runServer func() error) *cobra.Command {
	cmd := &cobra.Command{Use: "openbrain", SilenceUsage: true}
	client := &cliClient{client: &http.Client{Timeout: 30 * time.Second}}
	cmd.PersistentFlags().StringVar(&client.baseURL, "base-url", fallbackString(os.Getenv("OPENBRAIN_URL"), "http://localhost:3101"), "OpenBrain base URL")
	cmd.PersistentFlags().StringVar(&client.token, "token", fallbackString(os.Getenv("OPENBRAIN_API_KEY"), "dev_secret_token"), "OpenBrain API token")
	cmd.AddCommand(newServeCommand(runServer))
	cmd.AddCommand(newMemoryCommand(client))
	cmd.AddCommand(newAuditCommand(client))
	cmd.AddCommand(newHealthCommand(client))
	cmd.AddCommand(newWatchCommand(client))
	cmd.AddCommand(newTokenCommand())
	return cmd
}

func newServeCommand(runServer func() error) *cobra.Command {
	return &cobra.Command{Use: "serve", RunE: func(cmd *cobra.Command, args []string) error {
		if runServer == nil {
			return errors.New("server is not available in this context")
		}
		return runServer()
	}}
}

func newMemoryCommand(client *cliClient) *cobra.Command {
	memoryCmd := &cobra.Command{Use: "memory"}
	var namespace, entityType, content, title, query, proposalAction string
	var tier, limit int
	listCmd := &cobra.Command{Use: "list", RunE: func(cmd *cobra.Command, args []string) error {
		resp, err := client.doJSON(http.MethodGet, fmt.Sprintf("/api/v1/memories?namespaceId=%s&entityType=%s&limit=%d", namespace, entityType, limit), nil)
		if err != nil {
			return err
		}
		_, _ = io.Copy(cmd.OutOrStdout(), bytes.NewReader(resp))
		fmt.Fprintln(cmd.OutOrStdout())
		return nil
	}}
	listCmd.Flags().StringVar(&namespace, "namespace", "", "Namespace ID")
	listCmd.Flags().StringVar(&entityType, "type", "", "Entity type")
	listCmd.Flags().IntVar(&limit, "limit", 25, "Limit")
	_ = listCmd.MarkFlagRequired("namespace")

	getCmd := &cobra.Command{Use: "get <entity-id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		resp, err := client.doJSON(http.MethodGet, fmt.Sprintf("/api/v1/memories/%s?namespaceId=%s", args[0], namespace), nil)
		if err != nil {
			return err
		}
		_, _ = io.Copy(cmd.OutOrStdout(), bytes.NewReader(resp))
		fmt.Fprintln(cmd.OutOrStdout())
		return nil
	}}
	getCmd.Flags().StringVar(&namespace, "namespace", "", "Namespace ID")
	_ = getCmd.MarkFlagRequired("namespace")

	addCmd := &cobra.Command{Use: "add", RunE: func(cmd *cobra.Command, args []string) error {
		body := map[string]any{"namespaceId": namespace, "entityType": entityType, "title": title, "text": content, "tier": tier}
		resp, err := client.doJSON(http.MethodPost, "/api/v1/memories", body)
		if err != nil {
			return err
		}
		_, _ = io.Copy(cmd.OutOrStdout(), bytes.NewReader(resp))
		fmt.Fprintln(cmd.OutOrStdout())
		return nil
	}}
	addCmd.Flags().StringVar(&namespace, "namespace", "", "Namespace ID")
	addCmd.Flags().StringVar(&entityType, "type", "note", "Entity type")
	addCmd.Flags().StringVar(&title, "title", "", "Optional title")
	addCmd.Flags().StringVar(&content, "content", "", "Memory content")
	addCmd.Flags().IntVar(&tier, "tier", 0, "Memory tier")
	_ = addCmd.MarkFlagRequired("namespace")
	_ = addCmd.MarkFlagRequired("content")

	forgetCmd := &cobra.Command{Use: "forget <entity-id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		resp, err := client.doJSON(http.MethodDelete, fmt.Sprintf("/api/v1/memories/%s?namespaceId=%s", args[0], namespace), nil)
		if err != nil {
			return err
		}
		_, _ = io.Copy(cmd.OutOrStdout(), bytes.NewReader(resp))
		fmt.Fprintln(cmd.OutOrStdout())
		return nil
	}}
	forgetCmd.Flags().StringVar(&namespace, "namespace", "", "Namespace ID")
	_ = forgetCmd.MarkFlagRequired("namespace")

	searchCmd := &cobra.Command{Use: "search", RunE: func(cmd *cobra.Command, args []string) error {
		body := map[string]any{"namespaceId": namespace, "query": query, "topK": limit}
		resp, err := client.doJSON(http.MethodPost, "/api/v1/memories/search", body)
		if err != nil {
			return err
		}
		_, _ = io.Copy(cmd.OutOrStdout(), bytes.NewReader(resp))
		fmt.Fprintln(cmd.OutOrStdout())
		return nil
	}}
	searchCmd.Flags().StringVar(&namespace, "namespace", "", "Namespace ID")
	searchCmd.Flags().StringVar(&query, "query", "", "Search query")
	searchCmd.Flags().IntVar(&limit, "top-k", 10, "Top K")
	_ = searchCmd.MarkFlagRequired("namespace")
	_ = searchCmd.MarkFlagRequired("query")

	approveCmd := &cobra.Command{Use: "approve <proposal-id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		body := map[string]any{"action": fallbackString(proposalAction, "approve")}
		resp, err := client.doJSON(http.MethodPost, fmt.Sprintf("/v1/namespaces/%s/proposals/%s/resolve", namespace, args[0]), body)
		if err != nil {
			return err
		}
		_, _ = io.Copy(cmd.OutOrStdout(), bytes.NewReader(resp))
		fmt.Fprintln(cmd.OutOrStdout())
		return nil
	}}
	approveCmd.Flags().StringVar(&namespace, "namespace", "", "Namespace ID")
	approveCmd.Flags().StringVar(&proposalAction, "action", "approve", "approve or reject")
	_ = approveCmd.MarkFlagRequired("namespace")

	memoryCmd.AddCommand(listCmd, getCmd, addCmd, forgetCmd, searchCmd, approveCmd)
	return memoryCmd
}

func newAuditCommand(client *cliClient) *cobra.Command {
	var namespace, format, out string
	cmd := &cobra.Command{Use: "audit"}
	exportCmd := &cobra.Command{Use: "export", RunE: func(cmd *cobra.Command, args []string) error {
		resp, err := client.doJSON(http.MethodGet, fmt.Sprintf("/api/v1/audit/export?namespaceId=%s&format=%s", namespace, format), nil)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Exporting audit log in %s format to %s\n", format, out)
		return os.WriteFile(out, resp, 0o644)
	}}
	exportCmd.Flags().StringVar(&namespace, "namespace", "", "Namespace ID")
	exportCmd.Flags().StringVar(&format, "format", "jsonld", "Export format")
	exportCmd.Flags().StringVar(&out, "out", "./audit.jsonld", "Output file")
	_ = exportCmd.MarkFlagRequired("namespace")
	cmd.AddCommand(exportCmd)
	return cmd
}

func newHealthCommand(client *cliClient) *cobra.Command {
	var namespace string
	cmd := &cobra.Command{Use: "health", RunE: func(cmd *cobra.Command, args []string) error {
		path := "/api/v1/health"
		if namespace != "" {
			path += "?namespaceId=" + namespace
		}
		resp, err := client.doJSON(http.MethodGet, path, nil)
		if err != nil {
			return err
		}
		_, _ = io.Copy(cmd.OutOrStdout(), bytes.NewReader(resp))
		fmt.Fprintln(cmd.OutOrStdout())
		return nil
	}}
	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace ID")
	return cmd
}

func newWatchCommand(client *cliClient) *cobra.Command {
	var namespace, dir string
	var interval time.Duration
	cmd := &cobra.Command{Use: "watch", RunE: func(cmd *cobra.Command, args []string) error {
		if namespace == "" {
			return errors.New("namespace is required")
		}
		if dir == "" {
			dir = "."
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			resp, err := client.doJSON(http.MethodPost, fmt.Sprintf("/internal/v1/namespaces/%s/sync", namespace), map[string]any{"dir": dir})
			if err == nil {
				fmt.Fprintln(cmd.OutOrStdout(), string(resp))
			}
			select {
			case <-cmd.Context().Done():
				return nil
			case <-ticker.C:
			}
		}
	}}
	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace ID")
	cmd.Flags().StringVar(&dir, "dir", ".", "Directory to watch")
	cmd.Flags().DurationVar(&interval, "interval", 5*time.Second, "Polling interval")
	return cmd
}

func newTokenCommand() *cobra.Command {
	var namespace, agentID, actorKind, name string
	var trustTier int
	cmd := &cobra.Command{Use: "token", RunE: func(cmd *cobra.Command, args []string) error {
		token, err := brain.SignScopedToken(brain.ScopedTokenClaims{NamespaceID: namespace, AgentID: agentID, TrustTier: trustTier, ActorKind: actorKind, Name: name})
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), token)
		return nil
	}}
	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace ID")
	cmd.Flags().StringVar(&agentID, "agent-id", "", "Agent ID")
	cmd.Flags().StringVar(&actorKind, "actor-kind", "service", "Actor kind")
	cmd.Flags().StringVar(&name, "name", "", "Actor name")
	cmd.Flags().IntVar(&trustTier, "trust-tier", 4, "Trust tier")
	_ = cmd.MarkFlagRequired("namespace")
	return cmd
}

func Execute(runServer func() error) error {
	cliCmd = NewRootCommand(runServer)
	return cliCmd.Execute()
}

func (c *cliClient) doJSON(method, path string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}
	url := strings.TrimRight(c.baseURL, "/") + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("openbrain returned %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}

func fallbackString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
