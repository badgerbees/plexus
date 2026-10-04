package config

import (
	"bufio"
	"os"
	"strings"
)

type Config struct {
	JevAPIKey     string
	JudgeEndpoint string
	NEREndpoint   string

	OpenRouterAPIKey string
	JudgeModel       string

	// Generic OpenAI-compatible judge endpoint (e.g. a local Ollama
	// server at http://localhost:11434/v1).
	GenericJudgeEndpoint string
	GenericJudgeKey      string
}

func Load() Config {
	loadDotEnv(".env")

	return Config{
		JevAPIKey:     env("JEV_API_KEY", ""),
		JudgeEndpoint: env("JEV_JUDGE_ENDPOINT", ""),
		NEREndpoint:   env("PLEXUS_NER_ENDPOINT", ""),

		OpenRouterAPIKey: env("OPENROUTER_API_KEY", ""),
		JudgeModel:       env("JUDGE_MODEL", ""),

		GenericJudgeEndpoint: env("JUDGE_ENDPOINT", ""),
		GenericJudgeKey:      env("JUDGE_API_KEY", ""),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// JudgeProvider is the resolved judge wiring.
type JudgeProvider struct {
	Endpoint string
	APIKey   string
	Model    string
	Jev      bool
}

// Judge resolves the judge provider from config plus explicit overrides:
// an explicit Jev endpoint selects the Jev contract; a generic JUDGE_ENDPOINT
// (e.g. a local Ollama server) selects the chat-completions contract;
// otherwise OpenRouter defaults apply.
func (c Config) Judge(endpointOverride, keyOverride, modelOverride string) JudgeProvider {
	endpoint := endpointOverride
	if endpoint == "" {
		endpoint = c.JudgeEndpoint
	}
	if endpoint != "" {
		key := keyOverride
		if key == "" {
			key = c.JevAPIKey
		}
		return JudgeProvider{Endpoint: endpoint, APIKey: key, Jev: true}
	}

	endpoint = c.GenericJudgeEndpoint
	key := keyOverride
	if key == "" {
		key = c.GenericJudgeKey
		if key == "" {
			key = c.OpenRouterAPIKey
		}
	}
	model := modelOverride
	if model == "" {
		model = c.JudgeModel
	}
	return JudgeProvider{Endpoint: endpoint, APIKey: key, Model: model, Jev: false}
}

func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}
