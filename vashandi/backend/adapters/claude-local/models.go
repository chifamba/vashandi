package claudelocal

import "strings"

// Model definitions matching the Claude CLI expectations.
const (
	ModelSonnet = "claude-3-5-sonnet-20241022"
	ModelOpus   = "claude-3-opus-20240229"
	ModelHaiku  = "claude-3-5-haiku-20241022"
)

// ListModels returns the set of supported models for this adapter.
func ListModels() []string {
	return []string{
		ModelSonnet,
		ModelOpus,
		ModelHaiku,
	}
}

// IsBedrockModelID returns true if the model ID appears to be an AWS Bedrock native identifier.
func IsBedrockModelID(model string) bool {
	return strings.HasPrefix(model, "us.anthropic.") || 
		strings.Contains(model, ":") || 
		strings.HasPrefix(model, "arn:aws:bedrock:")
}
