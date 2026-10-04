package ingest

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"

	"plexus/internal/types"
)

type CSVReader struct {
	Path string
}

func (c *CSVReader) Read() ([]types.Document, error) {
	f, err := os.Open(c.Path)
	if err != nil {
		return nil, fmt.Errorf("open csv: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse csv: %w", err)
	}

	if len(records) < 2 {
		return nil, nil
	}

	headers := records[0]
	var docs []types.Document
	for i, row := range records[1:] {
		content := strings.Join(row, " | ")
		meta := make(map[string]string)
		for j, h := range headers {
			if j < len(row) {
				meta[h] = row[j]
			}
		}
		docs = append(docs, types.Document{
			ID:       fmt.Sprintf("csv-%d", i),
			Source:   c.Path,
			Format:   "csv",
			Content:  content,
			Metadata: meta,
		})
	}
	return docs, nil
}
