package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"plexus/internal/config"
	"plexus/internal/engine"
	"plexus/internal/ingest"
	"plexus/internal/types"
)

func main() {
	inputPath := flag.String("input", "", "path to input file or directory")
	outputPath := flag.String("output", "", "path to write anonymized output")
	format := flag.String("format", "auto", "input format: auto, slack, csv, text")
	decoys := flag.String("decoys", "", "comma-separated additional decoy terms")
	nerEndpoint := flag.String("ner-endpoint", "", "HTTP endpoint for Python NER microservice")
	judgeEndpoint := flag.String("judge-endpoint", "", "HTTP endpoint for LLM judge")
	judgeKey := flag.String("judge-key", "", "API key for judge endpoint")
	judgeModel := flag.String("judge-model", "", "model ID for the LLM judge")
	flag.Parse()

	cfg := config.Load()

	if *inputPath == "" {
		fmt.Fprintln(os.Stderr, "usage: plexus -input <file> [-output <file>] [-format auto|slack|csv|text]")
		os.Exit(1)
	}

	ner := coalesce(*nerEndpoint, cfg.NEREndpoint)

	judge := cfg.Judge(*judgeEndpoint, *judgeKey, *judgeModel)

	var pipe *engine.Pipeline
	var err error

	judgeCfg := engine.Config{
		NEREndpoint:   ner,
		JudgeEndpoint: judge.Endpoint,
		JudgeAPIKey:   judge.APIKey,
		JudgeModel:    judge.Model,
		JudgeJev:      judge.Jev,
	}
	if ner != "" {
		pipe, err = engine.NewWithConfig(judgeCfg)
		if err != nil {
			log.Fatalf("failed to init pipeline with model: %v", err)
		}
	} else {
		pipe, err = engine.NewWithConfig(judgeCfg)
		if err != nil {
			log.Fatalf("failed to init pipeline: %v", err)
		}
	}
	defer pipe.Close()

	if *decoys != "" {
		pipe.AddDecoys(strings.Split(*decoys, ",")...)
	}

	docs, err := loadDocuments(*inputPath, *format)
	if err != nil {
		log.Fatalf("failed to load documents: %v", err)
	}

	results, err := pipe.ProcessBatch(docs)
	if err != nil {
		log.Fatalf("processing failed: %v", err)
	}

	if *outputPath != "" {
		writeOutput(*outputPath, results)
	} else {
		for _, r := range results {
			fmt.Println(r)
		}
	}

	stats := pipe.GetStats()
	fmt.Fprintf(os.Stderr, "\n--- plexus stats ---\n")
	fmt.Fprintf(os.Stderr, "documents:  %d\n", stats.DocsProcessed)
	fmt.Fprintf(os.Stderr, "spans:      %d\n", stats.SpansDetected)
	fmt.Fprintf(os.Stderr, "accepted:   %d\n", stats.SpansAccepted)
	fmt.Fprintf(os.Stderr, "rejected:   %d\n", stats.SpansRejected)
	fmt.Fprintf(os.Stderr, "escalated:  %d\n", stats.SpansEscalated)
	fmt.Fprintf(os.Stderr, "judged:     %d\n", stats.SpansJudged)
}

func coalesce(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func loadDocuments(path, format string) ([]types.Document, error) {
	if format == "auto" {
		format = detectFormat(path)
	}

	switch format {
	case "slack":
		r := &ingest.SlackExport{Path: path}
		return r.Read()
	case "csv":
		r := &ingest.CSVReader{Path: path}
		return r.Read()
	case "text":
		return loadTextFile(path)
	default:
		return loadTextFile(path)
	}
}

func detectFormat(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".csv":
		return "csv"
	case ".json":
		return "slack"
	default:
		return "text"
	}
}

func loadTextFile(path string) ([]types.Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return []types.Document{{
		ID:      filepath.Base(path),
		Source:  path,
		Format:  "text",
		Content: string(data),
	}}, nil
}

func writeOutput(path string, results []string) {
	if strings.HasSuffix(path, ".json") {
		data, _ := json.MarshalIndent(results, "", "  ")
		os.WriteFile(path, data, 0644)
	} else {
		os.WriteFile(path, []byte(strings.Join(results, "\n---\n")), 0644)
	}
}
