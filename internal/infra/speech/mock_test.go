package speech

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestMock_TXT(t *testing.T) {
	m := NewMock(0)
	got, err := m.Transcribe(context.Background(), "meeting.txt", strings.NewReader("Обсудили бюджет."))
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if got != "Обсудили бюджет." {
		t.Fatalf("got %q", got)
	}
}

func TestMock_JSON(t *testing.T) {
	m := NewMock(0)
	got, err := m.Transcribe(context.Background(), "meeting.json",
		strings.NewReader(`{"transcription":"Из JSON"}`))
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if got != "Из JSON" {
		t.Fatalf("got %q", got)
	}
}

func TestMock_JSON_Fail(t *testing.T) {
	m := NewMock(0)
	_, err := m.Transcribe(context.Background(), "meeting.json",
		strings.NewReader(`{"fail":true,"fail_message":"boom"}`))
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMock_AudioStub(t *testing.T) {
	m := NewMock(0)
	got, err := m.Transcribe(context.Background(), "call.mp3", strings.NewReader(""))
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if !strings.Contains(got, "call.mp3") {
		t.Fatalf("stub should mention filename, got %q", got)
	}
}

func TestMock_RespectsContext(t *testing.T) {
	m := NewMock(5 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := m.Transcribe(ctx, "call.mp3", strings.NewReader(""))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
