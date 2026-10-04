package rewriter

import (
	"strings"
	"unicode"
)

type BlockType int

const (
	BlockPlainText BlockType = iota
	BlockCodeFenced
	BlockCodeInline
	BlockSQLQuery
	BlockJSONBody
	BlockShellCommand
)

type Block struct {
	Type    BlockType
	Content string
	Start   int
	End     int
}

func SegmentText(text string) []Block {
	var blocks []Block
	lines := strings.Split(text, "\n")

	pos := 0
	inFencedCode := false
	fenceStart := 0

	for i, line := range lines {
		lineStart := pos
		lineEnd := pos + len(line)
		if i < len(lines)-1 {
			lineEnd++
		}

		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			if !inFencedCode {
				if lineStart > fenceStart {
					blocks = appendBlock(blocks, Block{
						Type:    BlockPlainText,
						Content: text[fenceStart:lineStart],
						Start:   fenceStart,
						End:     lineStart,
					})
				}
				inFencedCode = true
				fenceStart = lineStart
			} else {
				blocks = append(blocks, Block{
					Type:    BlockCodeFenced,
					Content: text[fenceStart:lineEnd],
					Start:   fenceStart,
					End:     lineEnd,
				})
				inFencedCode = false
				fenceStart = lineEnd
			}
			pos = lineEnd
			continue
		}

		if !inFencedCode {
			if looksLikeSQL(trimmed) {
				if lineStart > fenceStart {
					blocks = appendBlock(blocks, Block{
						Type:    BlockPlainText,
						Content: text[fenceStart:lineStart],
						Start:   fenceStart,
						End:     lineStart,
					})
				}
				blocks = append(blocks, Block{
					Type:    BlockSQLQuery,
					Content: line,
					Start:   lineStart,
					End:     lineEnd,
				})
				fenceStart = lineEnd
			} else if looksLikeJSON(trimmed) {
				if lineStart > fenceStart {
					blocks = appendBlock(blocks, Block{
						Type:    BlockPlainText,
						Content: text[fenceStart:lineStart],
						Start:   fenceStart,
						End:     lineStart,
					})
				}
				blocks = append(blocks, Block{
					Type:    BlockJSONBody,
					Content: line,
					Start:   lineStart,
					End:     lineEnd,
				})
				fenceStart = lineEnd
			} else if looksLikeShell(trimmed) {
				if lineStart > fenceStart {
					blocks = appendBlock(blocks, Block{
						Type:    BlockPlainText,
						Content: text[fenceStart:lineStart],
						Start:   fenceStart,
						End:     lineStart,
					})
				}
				blocks = append(blocks, Block{
					Type:    BlockShellCommand,
					Content: line,
					Start:   lineStart,
					End:     lineEnd,
				})
				fenceStart = lineEnd
			}
		}

		pos = lineEnd
	}

	if fenceStart < len(text) {
		bt := BlockPlainText
		if inFencedCode {
			bt = BlockCodeFenced
		}
		blocks = appendBlock(blocks, Block{
			Type:    bt,
			Content: text[fenceStart:],
			Start:   fenceStart,
			End:     len(text),
		})
	}

	return blocks
}

func IsProtectedBlock(bt BlockType) bool {
	switch bt {
	case BlockCodeFenced, BlockCodeInline, BlockSQLQuery, BlockJSONBody, BlockShellCommand:
		return true
	}
	return false
}

func SpanInProtectedBlock(blocks []Block, start, end int) bool {
	for _, b := range blocks {
		if !IsProtectedBlock(b.Type) {
			continue
		}
		if start >= b.Start && end <= b.End {
			return true
		}
	}
	return false
}

func appendBlock(blocks []Block, b Block) []Block {
	if strings.TrimSpace(b.Content) == "" {
		return blocks
	}
	return append(blocks, b)
}

var sqlKeywords = []string{
	"SELECT ", "INSERT ", "UPDATE ", "DELETE ", "CREATE ", "ALTER ",
	"DROP ", "FROM ", "WHERE ", "JOIN ", "GROUP BY", "ORDER BY",
}

func looksLikeSQL(line string) bool {
	upper := strings.ToUpper(line)
	for _, kw := range sqlKeywords {
		if strings.HasPrefix(upper, kw) {
			return true
		}
	}
	return false
}

func looksLikeJSON(line string) bool {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 2 {
		return false
	}
	if (trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}') ||
		(trimmed[0] == '[' && trimmed[len(trimmed)-1] == ']') {
		braceCount := 0
		for _, r := range trimmed {
			if r == '{' || r == '[' {
				braceCount++
			}
		}
		return braceCount >= 1
	}
	if strings.HasPrefix(trimmed, "\"") && strings.Contains(trimmed, "\":") {
		return true
	}
	return false
}

func looksLikeShell(line string) bool {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "$ ") || strings.HasPrefix(trimmed, "# ") {
		rest := trimmed[2:]
		if len(rest) > 0 && unicode.IsLetter(rune(rest[0])) {
			return true
		}
	}
	shellPrefixes := []string{
		"sudo ", "docker ", "kubectl ", "git ", "npm ", "yarn ",
		"go ", "python ", "pip ", "curl ", "wget ", "ssh ",
		"export ", "source ", "chmod ", "chown ", "mkdir ",
	}
	for _, prefix := range shellPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}
