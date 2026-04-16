package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/client"
	"github.com/spf13/cobra"
)

var heartbeatCmd = &cobra.Command{
	Use:   "heartbeat",
	Short: "Heartbeat utilities",
}

var heartbeatRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Invoke an agent heartbeat and stream run events",
	RunE: func(cmd *cobra.Command, args []string) error {
		agentID, _ := cmd.Flags().GetString("agent-id")
		source, _ := cmd.Flags().GetString("source")
		trigger, _ := cmd.Flags().GetString("trigger")
		timeoutValue, _ := cmd.Flags().GetString("timeout-ms")
		follow, _ := cmd.Flags().GetBool("follow")
		jsonOutput, _ := cmd.Flags().GetBool("json")
		debug, _ := cmd.Flags().GetBool("debug")

		timeout, err := parseTimeout(timeoutValue)
		if err != nil {
			return err
		}

		apiClient := newAPIClient(cmd)
		agent, err := apiClient.GetAgent(context.Background(), agentID)
		if err != nil {
			return err
		}

		run, err := apiClient.WakeupHeartbeat(context.Background(), client.HeartbeatWakeupRequest{
			CompanyID:     agent.CompanyID,
			AgentID:       agent.ID,
			Source:        source,
			TriggerDetail: trigger,
		})
		if err != nil {
			return err
		}

		if jsonOutput {
			if err := printJSON(run); err != nil {
				return err
			}
		} else {
			fmt.Printf("Invoked heartbeat run %s for agent %s (%s) using adapter %s\n", run.ID, agent.Name, agent.ID, selectAdapter(agent.AdapterType))
		}

		finalRun, err := streamHeartbeat(context.Background(), apiClient, run.ID, agent.AdapterType, follow, timeout, jsonOutput, debug)
		if err != nil {
			return err
		}

		if finalRun == nil {
			return nil
		}
		if jsonOutput {
			return printJSON(finalRun)
		}
		if finalRun.Status == "completed" {
			fmt.Printf("Run %s completed successfully\n", finalRun.ID)
			return nil
		}
		printHeartbeatFailure(finalRun)
		return fmt.Errorf("heartbeat run %s finished with status %s", finalRun.ID, finalRun.Status)
	},
}

func init() {
	addAPIFlags(heartbeatRunCmd)
	heartbeatRunCmd.Flags().StringP("agent-id", "a", "", "Agent ID to invoke")
	heartbeatRunCmd.Flags().String("context", "", "Path to CLI context file")
	heartbeatRunCmd.Flags().String("profile", "", "CLI context profile name")
	heartbeatRunCmd.Flags().String("source", "on_demand", "Invocation source (timer | assignment | on_demand | automation)")
	heartbeatRunCmd.Flags().String("trigger", "manual", "Trigger detail (manual | ping | callback | system)")
	heartbeatRunCmd.Flags().String("timeout-ms", "0", "Max time to wait before giving up")
	heartbeatRunCmd.Flags().Bool("follow", false, "Poll events until the run reaches a terminal state")
	heartbeatRunCmd.Flags().Bool("json", false, "Output raw JSON where applicable")
	heartbeatRunCmd.Flags().Bool("debug", false, "Show raw adapter stdout/stderr JSON chunks")

	heartbeatRunCmd.MarkFlagRequired("agent-id")

	heartbeatCmd.AddCommand(heartbeatRunCmd)
	rootCmd.AddCommand(heartbeatCmd)
}

func streamHeartbeat(ctx context.Context, apiClient *client.Client, runID, adapterType string, follow bool, timeout time.Duration, jsonOutput, debug bool) (*client.HeartbeatRun, error) {
	deadline := time.Time{}
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}

	lastSeq := 0
	lastStatus := ""

	for {
		events, err := apiClient.ListHeartbeatRunEvents(ctx, runID, lastSeq, 200)
		if err != nil {
			return nil, err
		}
		for _, event := range events {
			if event.Seq > lastSeq {
				lastSeq = event.Seq
			}
			printHeartbeatEvent(event, adapterType, jsonOutput, debug)
		}

		run, err := apiClient.GetHeartbeatRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		if run.Status != "" && run.Status != lastStatus {
			if jsonOutput {
				if err := printJSON(map[string]string{"runId": runID, "status": run.Status}); err != nil {
					return nil, err
				}
			} else {
				fmt.Printf("[status] %s\n", run.Status)
			}
			lastStatus = run.Status
		}

		if !follow || isTerminalHeartbeatStatus(run.Status) {
			return run, nil
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return nil, fmt.Errorf("heartbeat run %s timed out after %s", runID, timeout)
		}

		time.Sleep(250 * time.Millisecond)
	}
}

func printHeartbeatEvent(event client.HeartbeatRunEvent, adapterType string, jsonOutput, debug bool) {
	if jsonOutput {
		_ = printJSON(event)
		return
	}

	switch event.EventType {
	case "adapter.invoke":
		printAdapterInvoke(adapterType, event.Payload)
	case "heartbeat.run.status":
		if status := stringValue(event.Payload["status"]); status != "" {
			fmt.Printf("[status] %s\n", status)
		} else if event.Message != nil && *event.Message != "" {
			fmt.Printf("[status] %s\n", *event.Message)
		}
	case "heartbeat.run.log":
		stream := stringValue(event.Payload["stream"])
		if stream == "" && event.Stream != nil {
			stream = *event.Stream
		}
		if stream == "" {
			stream = "system"
		}
		chunk := stringValue(event.Payload["chunk"])
		if chunk == "" && event.Message != nil {
			chunk = *event.Message
		}
		printLogChunk(stream, chunk, debug)
	default:
		if event.Message != nil && *event.Message != "" {
			fmt.Printf("[event] %s: %s\n", event.EventType, *event.Message)
		}
	}
}

func printAdapterInvoke(adapterType string, payload map[string]interface{}) {
	selectedAdapter := stringValue(payload["adapterType"])
	if selectedAdapter == "" {
		selectedAdapter = selectAdapter(adapterType)
	}
	fmt.Printf("Adapter: %s\n", selectedAdapter)
	if cwd := stringValue(payload["cwd"]); cwd != "" {
		fmt.Printf("Working dir: %s\n", cwd)
	}
	if command := stringValue(payload["command"]); command != "" {
		commandArgs := stringSlice(payload["commandArgs"])
		if len(commandArgs) > 0 {
			fmt.Printf("Command: %s %s\n", command, strings.Join(commandArgs, " "))
		} else {
			fmt.Printf("Command: %s\n", command)
		}
	}
	if prompt := stringValue(payload["prompt"]); prompt != "" {
		fmt.Printf("Prompt:\n%s\n", prompt)
	}
}

func printLogChunk(stream, chunk string, debug bool) {
	if chunk == "" {
		return
	}
	label := "[" + stream + "] "
	if stream == "stdout" && !debug {
		fmt.Print(chunk)
		return
	}
	fmt.Print(label + chunk)
}

func printHeartbeatFailure(run *client.HeartbeatRun) {
	if run.Error != nil && *run.Error != "" {
		fmt.Printf("Error: %s\n", *run.Error)
	}
	if run.StderrExcerpt != nil && *run.StderrExcerpt != "" {
		fmt.Println("stderr excerpt:")
		fmt.Println(*run.StderrExcerpt)
	}
	if run.StdoutExcerpt != nil && *run.StdoutExcerpt != "" {
		fmt.Println("stdout excerpt:")
		fmt.Println(*run.StdoutExcerpt)
	}
	if len(run.ResultJSON) > 0 {
		encoded, err := json.MarshalIndent(run.ResultJSON, "", "  ")
		if err == nil {
			fmt.Println("result:")
			fmt.Println(string(encoded))
		}
	}
}

func parseTimeout(value string) (time.Duration, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "0" {
		return 0, nil
	}
	millis, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid timeout-ms value %q", value)
	}
	if millis < 0 {
		return 0, fmt.Errorf("timeout-ms must be non-negative")
	}
	return time.Duration(millis) * time.Millisecond, nil
}

func isTerminalHeartbeatStatus(status string) bool {
	switch status {
	case "completed", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func selectAdapter(adapterType string) string {
	switch strings.TrimSpace(adapterType) {
	case "":
		return "process"
	case "claude", "claude_local":
		return "claude"
	case "openai":
		return "openai"
	case "gemini":
		return "gemini"
	case "codex", "codex_local":
		return "codex"
	default:
		return adapterType
	}
}

func stringValue(value interface{}) string {
	if str, ok := value.(string); ok {
		return str
	}
	return ""
}

func stringSlice(value interface{}) []string {
	values, ok := value.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, entry := range values {
		if str, ok := entry.(string); ok {
			result = append(result, str)
		}
	}
	return result
}
