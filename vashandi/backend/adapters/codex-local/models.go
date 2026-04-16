package codexlocal

const (
	ModelCodexBase  = "codex-base"
	ModelCodexSmart = "codex-smart"
	ModelCodexFast  = "codex-fast"
)

// ListModels returns a default list of supported models.
func ListModels() []string {
	return []string{
		ModelCodexBase,
		ModelCodexSmart,
		ModelCodexFast,
	}
}
