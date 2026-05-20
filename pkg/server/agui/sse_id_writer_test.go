package agui

import (
	"testing"
)

func TestStripIDLine_RemovesSDKTimestampID(t *testing.T) {
	frame := []byte("event: RUN_STARTED\nid: RUN_STARTED_1716234567890\ndata: {\"type\":\"RUN_STARTED\"}\n\n")
	got := stripIDLine(frame)
	expected := "event: RUN_STARTED\ndata: {\"type\":\"RUN_STARTED\"}\n\n"
	if string(got) != expected {
		t.Errorf("stripIDLine:\ngot:  %q\nwant: %q", string(got), expected)
	}
}

func TestStripIDLine_NoIDLine(t *testing.T) {
	frame := []byte("event: TEXT\ndata: {\"text\":\"hello\"}\n\n")
	got := stripIDLine(frame)
	if string(got) != string(frame) {
		t.Errorf("stripIDLine should not modify frame without id:\ngot:  %q\nwant: %q", string(got), string(frame))
	}
}

func TestStripIDLine_MultipleIDLines(t *testing.T) {
	frame := []byte("id: first\nevent: X\nid: second\ndata: {}\n\n")
	got := stripIDLine(frame)
	expected := "event: X\ndata: {}\n\n"
	if string(got) != expected {
		t.Errorf("stripIDLine:\ngot:  %q\nwant: %q", string(got), expected)
	}
}

func TestIDOverrideWriter(t *testing.T) {
	var buf []byte
	w := &idOverrideWriter{w: &byteWriter{buf: &buf}}

	input := []byte("event: RUN_STARTED\nid: RUN_STARTED_123\ndata: {}\n\n")
	n, err := w.Write(input)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(input) {
		t.Errorf("Write returned %d, want %d", n, len(input))
	}

	expected := "event: RUN_STARTED\ndata: {}\n\n"
	if string(buf) != expected {
		t.Errorf("idOverrideWriter:\ngot:  %q\nwant: %q", string(buf), expected)
	}
}

type byteWriter struct {
	buf *[]byte
}

func (w *byteWriter) Write(p []byte) (int, error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}
