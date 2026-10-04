package ingest

import (
	"encoding/json"
	"fmt"
	"os"

	"plexus/internal/types"
)

type SlackExport struct {
	Path string
}

type slackMessage struct {
	User    string `json:"user"`
	Text    string `json:"text"`
	TS      string `json:"ts"`
	Channel string `json:"channel"`
}

func (s *SlackExport) Read() ([]types.Document, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		return nil, fmt.Errorf("read slack export: %w", err)
	}

	var messages []slackMessage
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, fmt.Errorf("parse slack export: %w", err)
	}

	docs := make([]types.Document, len(messages))
	for i, msg := range messages {
		docs[i] = types.Document{
			ID:      fmt.Sprintf("slack-%s-%s", msg.Channel, msg.TS),
			Source:  s.Path,
			Format:  "slack",
			Content: msg.Text,
			Metadata: map[string]string{
				"user":    msg.User,
				"channel": msg.Channel,
				"ts":      msg.TS,
			},
		}
	}
	return docs, nil
}
