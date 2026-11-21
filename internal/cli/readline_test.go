package cli

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReadLine_LimitIsExact(t *testing.T) {
	const max = 10
	input := strings.Repeat("a", max) + "\r\n" + strings.Repeat("b", max+1) + "\n" + "tail"
	r := bufio.NewReaderSize(strings.NewReader(input), 16)

	tests := []struct {
		line    string
		tooLong bool
	}{
		{strings.Repeat("a", max), false}, // exactly the limit: accepted, "\r\n" removed
		{"", true},                        // one byte over: rejected
		{"tail", false},                   // last line without a newline
	}
	for i, want := range tests {
		line, tooLong, err := readLine(r, max)
		if err != nil || line != want.line || tooLong != want.tooLong {
			t.Fatalf("line %d: got (%q, %v, %v), want (%q, %v, nil)", i, line, tooLong, err, want.line, want.tooLong)
		}
	}
	if _, _, err := readLine(r, max); !errors.Is(err, io.EOF) {
		t.Fatalf("after the last line: err = %v, want io.EOF", err)
	}
}
