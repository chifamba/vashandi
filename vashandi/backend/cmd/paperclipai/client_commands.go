package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/chifamba/vashandi/vashandi/backend/client"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(
		newCompanyClientCommand(),
		newIssueClientCommand(),
		newAgentClientCommand(),
		newApprovalClientCommand(),
		newActivityClientCommand(),
		newDashboardClientCommand(),
		newPluginClientCommand(),
		newContextClientCommand(),
	)
}

func newCompanyClientCommand() *cobra.Command {
	cmd := namespaceCommand("company", "Company client commands")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List companies",
		RunE: func(cmd *cobra.Command, args []string) error {
			apiClient := newAPIClient(cmd)
			companies, err := apiClient.ListCompanies(context.Background())
			if err != nil {
				return err
			}
			return printJSON(companies)
		},
	}
	addAPIFlags(listCmd)

	getCmd := &cobra.Command{
		Use:   "get <company-id>",
		Short: "Get a company",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiClient := newAPIClient(cmd)
			company, err := apiClient.GetCompany(context.Background(), args[0])
			if err != nil {
				return err
			}
			return printJSON(company)
		},
	}
	addAPIFlags(getCmd)

	cmd.AddCommand(listCmd, getCmd)
	return cmd
}

func newIssueClientCommand() *cobra.Command {
	cmd := namespaceCommand("issue", "Issue client commands")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List issues for a company",
		RunE: func(cmd *cobra.Command, args []string) error {
			companyID, err := resolveRequiredCompanyID(cmd)
			if err != nil {
				return err
			}
			apiClient := newAPIClient(cmd)
			issues, err := apiClient.ListIssues(context.Background(), companyID)
			if err != nil {
				return err
			}
			return printJSON(issues)
		},
	}
	addAPIFlags(listCmd)
	addCompanyFlag(listCmd)

	getCmd := &cobra.Command{
		Use:   "get <issue-id>",
		Short: "Get an issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiClient := newAPIClient(cmd)
			issue, err := apiClient.GetIssue(context.Background(), args[0])
			if err != nil {
				return err
			}
			return printJSON(issue)
		},
	}
	addAPIFlags(getCmd)

	cmd.AddCommand(listCmd, getCmd)
	return cmd
}

func newAgentClientCommand() *cobra.Command {
	cmd := namespaceCommand("agent", "Agent client commands")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List agents for a company",
		RunE: func(cmd *cobra.Command, args []string) error {
			companyID, err := resolveRequiredCompanyID(cmd)
			if err != nil {
				return err
			}
			apiClient := newAPIClient(cmd)
			agents, err := apiClient.ListAgents(context.Background(), companyID)
			if err != nil {
				return err
			}
			return printJSON(agents)
		},
	}
	addAPIFlags(listCmd)
	addCompanyFlag(listCmd)

	getCmd := &cobra.Command{
		Use:   "get <agent-id>",
		Short: "Get an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiClient := newAPIClient(cmd)
			agent, err := apiClient.GetAgent(context.Background(), args[0])
			if err != nil {
				return err
			}
			return printJSON(agent)
		},
	}
	addAPIFlags(getCmd)

	cmd.AddCommand(listCmd, getCmd)
	return cmd
}

func newApprovalClientCommand() *cobra.Command {
	cmd := namespaceCommand("approval", "Approval client commands")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List approvals for a company",
		RunE: func(cmd *cobra.Command, args []string) error {
			companyID, err := resolveRequiredCompanyID(cmd)
			if err != nil {
				return err
			}
			status, _ := cmd.Flags().GetString("status")
			apiClient := newAPIClient(cmd)
			approvals, err := apiClient.ListApprovals(context.Background(), companyID, status)
			if err != nil {
				return err
			}
			return printJSON(approvals)
		},
	}
	addAPIFlags(listCmd)
	addCompanyFlag(listCmd)
	listCmd.Flags().String("status", "", "Optional approval status filter")

	getCmd := &cobra.Command{
		Use:   "get <approval-id>",
		Short: "Get an approval",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiClient := newAPIClient(cmd)
			approval, err := apiClient.GetApproval(context.Background(), args[0])
			if err != nil {
				return err
			}
			return printJSON(approval)
		},
	}
	addAPIFlags(getCmd)

	cmd.AddCommand(listCmd, getCmd)
	return cmd
}

func newActivityClientCommand() *cobra.Command {
	cmd := namespaceCommand("activity", "Activity client commands")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List company activity entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			companyID, err := resolveRequiredCompanyID(cmd)
			if err != nil {
				return err
			}
			opts := client.ActivityListOptions{}
			opts.AgentID, _ = cmd.Flags().GetString("agent-id")
			opts.EntityType, _ = cmd.Flags().GetString("entity-type")
			opts.EntityID, _ = cmd.Flags().GetString("entity-id")

			apiClient := newAPIClient(cmd)
			activity, err := apiClient.ListActivity(context.Background(), companyID, opts)
			if err != nil {
				return err
			}
			return printJSON(activity)
		},
	}
	addAPIFlags(listCmd)
	addCompanyFlag(listCmd)
	listCmd.Flags().String("agent-id", "", "Optional agent ID filter")
	listCmd.Flags().String("entity-type", "", "Optional entity type filter")
	listCmd.Flags().String("entity-id", "", "Optional entity ID filter")

	getCmd := &cobra.Command{
		Use:   "get <activity-id>",
		Short: "Get an activity entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiClient := newAPIClient(cmd)
			activity, err := apiClient.GetActivity(context.Background(), args[0])
			if err != nil {
				return err
			}
			return printJSON(activity)
		},
	}
	addAPIFlags(getCmd)

	cmd.AddCommand(listCmd, getCmd)
	return cmd
}

func newDashboardClientCommand() *cobra.Command {
	cmd := namespaceCommand("dashboard", "Dashboard client commands")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "Get platform metrics",
		RunE: func(cmd *cobra.Command, args []string) error {
			apiClient := newAPIClient(cmd)
			metrics, err := apiClient.GetPlatformMetrics(context.Background())
			if err != nil {
				return err
			}
			return printJSON(metrics)
		},
	}
	addAPIFlags(listCmd)

	getCmd := &cobra.Command{
		Use:   "get",
		Short: "Get dashboard summary for a company",
		RunE: func(cmd *cobra.Command, args []string) error {
			companyID, err := resolveRequiredCompanyID(cmd)
			if err != nil {
				return err
			}
			apiClient := newAPIClient(cmd)
			summary, err := apiClient.GetDashboard(context.Background(), companyID)
			if err != nil {
				return err
			}
			return printJSON(summary)
		},
	}
	addAPIFlags(getCmd)
	addCompanyFlag(getCmd)

	cmd.AddCommand(listCmd, getCmd)
	return cmd
}

func newPluginClientCommand() *cobra.Command {
	cmd := namespaceCommand("plugin", "Plugin client commands")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List plugins",
		RunE: func(cmd *cobra.Command, args []string) error {
			status, _ := cmd.Flags().GetString("status")
			apiClient := newAPIClient(cmd)
			plugins, err := apiClient.ListPlugins(context.Background(), status)
			if err != nil {
				return err
			}
			return printJSON(plugins)
		},
	}
	addAPIFlags(listCmd)
	listCmd.Flags().String("status", "", "Optional plugin status filter")

	getCmd := &cobra.Command{
		Use:   "get <plugin-id>",
		Short: "Get a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiClient := newAPIClient(cmd)
			plugin, err := apiClient.GetPlugin(context.Background(), args[0])
			if err != nil {
				return err
			}
			return printJSON(plugin)
		},
	}
	addAPIFlags(getCmd)

	cmd.AddCommand(listCmd, getCmd)
	return cmd
}

func newContextClientCommand() *cobra.Command {
	cmd := namespaceCommand("context", "Context client commands")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List available company context operations",
		RunE: func(cmd *cobra.Command, args []string) error {
			companyID, err := resolveRequiredCompanyID(cmd)
			if err != nil {
				return err
			}
			apiClient := newAPIClient(cmd)
			operations, err := apiClient.ListContextOperations(context.Background(), companyID)
			if err != nil {
				return err
			}
			return printJSON(operations)
		},
	}
	addAPIFlags(listCmd)
	addCompanyFlag(listCmd)

	getCmd := &cobra.Command{
		Use:   "get <operation>",
		Short: "Get one company context operation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			companyID, err := resolveRequiredCompanyID(cmd)
			if err != nil {
				return err
			}
			apiClient := newAPIClient(cmd)
			operation, err := apiClient.GetContextOperation(context.Background(), companyID, args[0])
			if err != nil {
				return err
			}
			return printJSON(operation)
		},
	}
	addAPIFlags(getCmd)
	addCompanyFlag(getCmd)

	cmd.AddCommand(listCmd, getCmd)
	return cmd
}

func namespaceCommand(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
}

func addAPIFlags(cmd *cobra.Command) {
	cmd.Flags().String("api-base", "", "Base URL for the Paperclip server API")
	cmd.Flags().String("api-key", "", "Bearer token for API requests")
}

func addCompanyFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("company", "c", "", "Company ID")
}

func newAPIClient(cmd *cobra.Command) *client.Client {
	cfg := loadConfig()
	baseURL, _ := cmd.Flags().GetString("api-base")
	if baseURL == "" {
		baseURL = cfg.ServerURL
	}

	apiKey, _ := cmd.Flags().GetString("api-key")
	if apiKey == "" {
		apiKey = cfg.APIKey
	}
	if apiKey == "" {
		apiKey = os.Getenv("PAPERCLIP_AGENT_JWT_SECRET")
	}
	if apiKey == "" {
		apiKey = os.Getenv("PAPERCLIP_API_KEY")
	}

	return client.NewClient(baseURL, apiKey)
}

func resolveRequiredCompanyID(cmd *cobra.Command) (string, error) {
	companyID, _ := cmd.Flags().GetString("company")
	if companyID == "" {
		companyID = loadConfig().DefaultCompany
	}
	if companyID == "" {
		return "", fmt.Errorf("company ID is required")
	}
	return companyID, nil
}

func printJSON(value interface{}) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}
