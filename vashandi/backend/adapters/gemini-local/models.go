package geminilocal

const (
	ModelGemini20Flash = "gemini-2.0-flash"
	ModelGemini15Pro   = "gemini-1.5-pro"
	ModelGemini15Flash = "gemini-1.5-flash"
)

func ListModels() []string {
	return []string{
		ModelGemini20Flash,
		ModelGemini15Pro,
		ModelGemini15Flash,
	}
}
