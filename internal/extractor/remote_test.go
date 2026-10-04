package extractor

import "testing"

// Regression test for the character/byte offset mismatch: the Python NER
// service reports character offsets, while Go indexes strings by bytes.
// Non-ASCII text (multi-byte runes) previously caused replacements to land
// on the wrong bytes. charOffsetToByte must map rune offsets to byte offsets.
func TestCharOffsetToByte(t *testing.T) {
	cases := []struct {
		s        string
		char     int
		wantByte int
	}{
		{"", 0, 0},
		{"abc", 1, 1},
		{"abc", 3, 3},
		{"abc", 99, 3}, // clamp past end
		{"héllo", 1, 1},
		{"héllo", 2, 3}, // 'é' is 2 bytes in UTF-8
		{"héllo", 5, 6}, // 5 chars = 6 bytes
		{"日本語", 1, 3},
		{"日本語", 3, 9},
		{"a日b", 2, 4}, // a(1) + 日(3) -> byte offset 4
	}
	for _, c := range cases {
		if got := charOffsetToByte(c.s, c.char); got != c.wantByte {
			t.Errorf("charOffsetToByte(%q, %d) = %d, want %d", c.s, c.char, got, c.wantByte)
		}
	}
}
