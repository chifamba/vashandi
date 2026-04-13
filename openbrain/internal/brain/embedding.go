package brain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"os"
	"strings"
	"time"
)

// EmbeddingProvider defines the interface for generating vector embeddings
type EmbeddingProvider interface {
	GenerateEmbedding(ctx context.Context, text string) ([]float64, error)
	Dimensions() int
}

// --- Stub Provider (FNV based, default 1536d) ---

type StubProvider struct {
	dims int
}

func NewStubProvider(dims int) *StubProvider {
	if dims <= 0 {
		dims = 1536
	}
	return &StubProvider{dims: dims}
}

func (p *StubProvider) Dimensions() int {
	return p.dims
}

func (p *StubProvider) GenerateEmbedding(ctx context.Context, text string) ([]float64, error) {
	vec := make([]float64, p.dims)
	// Simple 1D-tokenize (same as the previous internal function)
	tokens := strings.Fields(strings.ToLower(text))
	for _, token := range tokens {
		h := fnv.New64a()
		_, _ = h.Write([]byte(token))
		sum := h.Sum64()
		idx := int(sum % uint64(p.dims))
		sign := 1.0
		if sum&1 == 1 {
			sign = -1.0
		}
		vec[idx] += sign
	}
	// L2 Normalize
	norm := 0.0
	for _, v := range vec {
		norm += v * v
	}
	if norm > 0 {
		sr := 1.0 / (norm * norm) // Simplified normalization
		for i := range vec {
			vec[i] *= sr
		}
	}
	return vec, nil
}

// --- OpenAI Provider ---

type OpenAIProvider struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewOpenAIProvider(apiKey, model string) *OpenAIProvider {
	if model == "" {
		model = "text-embedding-3-small"
	}
	return &OpenAIProvider{
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *OpenAIProvider) Dimensions() int {
	// OpenAI text-embedding-3-small is 1536, large is 3072.
	// For now we assume standard 1536 for our schema.
	return 1536
}

type openAIResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func (p *OpenAIProvider) GenerateEmbedding(ctx context.Context, text string) ([]float64, error) {
	url := "https://api.openai.com/v1/embeddings"
	payload := map[string]interface{}{
		"input": text,
		"model": p.model,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai returned status %d", resp.StatusCode)
	}

	var res openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	if len(res.Data) == 0 {
		return nil, fmt.Errorf("openai returned no embedding data")
	}

	return res.Data[0].Embedding, nil
}

// --- Ollama Provider ---

type OllamaProvider struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

func NewOllamaProvider(baseURL, model string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "nomic-embed-text"
	}
	return &OllamaProvider{
		baseURL:    baseURL,
		model:      model,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *OllamaProvider) Dimensions() int {
	// Ollama dimensions vary by model. nomic-embed-text is 768.
	// WARNING: Switching dimensions requires database schema updates.
	return 1536 // Placeholder - we strongly recommend 1536 consistency.
}

func (p *OllamaProvider) GenerateEmbedding(ctx context.Context, text string) ([]float64, error) {
	url := p.baseURL + "/api/embeddings"
	payload := map[string]interface{}{
		"model":  p.model,
		"prompt": text,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var res struct {
		Embedding []float64 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	return res.Embedding, nil
}

// InitEmbeddingProvider initializes a provider based on environment variables
func InitEmbeddingProvider() EmbeddingProvider {
	provider := os.Getenv("EMBEDDING_PROVIDER")
	switch strings.ToLower(provider) {
	case "openai":
		return NewOpenAIProvider(os.Getenv("OPENAI_API_KEY"), os.Getenv("OPENAI_EMBEDDING_MODEL"))
	case "ollama":
		return NewOllamaProvider(os.Getenv("OLLAMA_BASE_URL"), os.Getenv("OLLAMA_EMBEDDING_MODEL"))
	default:
		return NewStubProvider(1536)
	}
}
