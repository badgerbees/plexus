package extractor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"plexus/internal/types"
)

type RemoteExtractor struct {
	endpoints []string
	client    *http.Client
	counter   atomic.Uint64
}

type extractReq struct {
	Text       string `json:"text"`
	SourceFile string `json:"source_file"`
}

type extractResp struct {
	Spans []types.CandidateSpan `json:"spans"`
}

func NewRemoteExtractor(endpoint string) *RemoteExtractor {
	return NewRemoteExtractorPool(strings.Split(endpoint, ","))
}

// NewRemoteExtractorPool round-robins requests across multiple NER server
// instances (e.g. "http://127.0.0.1:5000/extract,http://127.0.0.1:5001/extract")
// so concurrent documents can be served in parallel.
func NewRemoteExtractorPool(endpoints []string) *RemoteExtractor {
	trimmed := make([]string, 0, len(endpoints))
	for _, e := range endpoints {
		if e = strings.TrimSpace(e); e != "" {
			trimmed = append(trimmed, e)
		}
	}
	return &RemoteExtractor{
		endpoints: trimmed,
		client:    &http.Client{Timeout: 120 * time.Second},
	}
}

func (r *RemoteExtractor) nextEndpoint() string {
	if len(r.endpoints) == 0 {
		return ""
	}
	if len(r.endpoints) == 1 {
		return r.endpoints[0]
	}
	return r.endpoints[r.counter.Add(1)%uint64(len(r.endpoints))]
}

func (r *RemoteExtractor) Extract(text, sourceFile string) ([]types.CandidateSpan, error) {
	reqBody := extractReq{
		Text:       text,
		SourceFile: sourceFile,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal req: %w", err)
	}

	req, err := http.NewRequest("POST", r.nextEndpoint(), bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("new req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do req: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bad status %d: %s", resp.StatusCode, string(body))
	}

	var result extractResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode resp: %w", err)
	}

	// The Python service reports character offsets into the text, while Go
	// indexes strings by bytes. Convert so replacements line up for any
	// text containing non-ASCII characters.
	for i := range result.Spans {
		result.Spans[i].StartByte = charOffsetToByte(text, result.Spans[i].StartByte)
		result.Spans[i].EndByte = charOffsetToByte(text, result.Spans[i].EndByte)
	}

	return result.Spans, nil
}

// charOffsetToByte converts a character (rune) offset into `s` to its byte
// offset, clamping to the byte length of the string.
func charOffsetToByte(s string, charOffset int) int {
	if charOffset <= 0 {
		return 0
	}
	for i := range s {
		charOffset--
		if charOffset == 0 {
			_, size := utf8.DecodeRuneInString(s[i:])
			return i + size
		}
	}
	return len(s)
}
