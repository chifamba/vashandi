package services

import (
	"context"
	"fmt"
	"gorm.io/gorm"
)

type PluginToolDispatcher struct {
	DB            *gorm.DB
	Registry      *PluginToolRegistry
	WorkerManager *PluginWorkerManager
}

func NewPluginToolDispatcher(db *gorm.DB, registry *PluginToolRegistry, wm *PluginWorkerManager) *PluginToolDispatcher {
	return &PluginToolDispatcher{
		DB:            db,
		Registry:      registry,
		WorkerManager: wm,
	}
}

type AgentToolDescriptor struct {
	Name             string                 `json:"name"`
	DisplayName      string                 `json:"displayName"`
	Description      string                 `json:"description"`
	ParametersSchema map[string]interface{} `json:"parametersSchema"`
	PluginID         string                 `json:"pluginId"`
}

func (d *PluginToolDispatcher) ListToolsForAgent(ctx context.Context, agentID, companyID string) ([]AgentToolDescriptor, error) {
	// For now, we return all registered tools.
	// In the future, this will filter by agent permissions and plugin company scope.
	tools := d.Registry.ListTools("")
	
	result := make([]AgentToolDescriptor, 0, len(tools))
	for _, t := range tools {
		result = append(result, AgentToolDescriptor{
			Name:             t.NamespacedName,
			DisplayName:      t.DisplayName,
			Description:      t.Description,
			ParametersSchema: t.ParametersSchema,
			PluginID:         t.PluginDbID,
		})
	}
	return result, nil
}

type ToolExecutionParams struct {
	Tool       string                 `json:"tool"` // Namespaced name
	Parameters map[string]interface{} `json:"parameters"`
	RunContext map[string]interface{} `json:"runContext"`
}

func (d *PluginToolDispatcher) ExecuteTool(ctx context.Context, namespacedName string, parameters interface{}, runContext interface{}) (interface{}, error) {
	// 1. Resolve tool metadata
	tool := d.Registry.GetTool(namespacedName)
	if tool == nil {
		return nil, fmt.Errorf("tool %q not found or plugin not ready", namespacedName)
	}

	// 2. Ensure worker is running
	if !d.WorkerManager.IsRunning(tool.PluginDbID) {
		return nil, fmt.Errorf("worker for plugin %q is not running", tool.PluginID)
	}

	// 3. Dispatch RPC call
	rpcParams := map[string]interface{}{
		"toolName":   tool.Name, // Bare name for worker
		"parameters": parameters,
		"runContext": runContext,
	}

	res, err := d.WorkerManager.Call(ctx, tool.PluginDbID, "executeTool", rpcParams, 0)
	if err != nil {
		return nil, err
	}

	return res, nil
}
