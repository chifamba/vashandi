package cursorlocal

const (
	ModelGPT4o        = "gpt-4o"
	ModelGPT4Turbo    = "gpt-4-turbo"
	ModelClaudeSonnet = "claude-3-5-sonnet-20241022"
)

// ListModels returns a default list of supported models in Cursor.
func ListModels() []string {
	return []string{
		ModelGPT4o,
		ModelGPT4Turbo,
		ModelClaudeSonnet,
	}
}
