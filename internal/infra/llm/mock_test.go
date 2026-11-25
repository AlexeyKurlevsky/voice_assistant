package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestMock_Summarize(t *testing.T) {
	m := NewMock(0)
	got, err := m.Summarize(context.Background(), "Обсудили бюджет и сроки релиза.")
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if !strings.HasPrefix(got, "Краткая выжимка: ") {
		t.Fatalf("got %q", got)
	}
}

func TestMock_Summarize_Empty(t *testing.T) {
	m := NewMock(0)
	if _, err := m.Summarize(context.Background(), "   "); err == nil {
		t.Fatal("expected error for empty transcript")
	}
}

func TestMock_Answer(t *testing.T) {
	m := NewMock(0)
	got, err := m.Answer(context.Background(), "какие сроки?", "контекст")
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if !strings.Contains(got, "какие сроки?") {
		t.Fatalf("answer should echo question, got %q", got)
	}
}

func TestMock_Answer_Empty(t *testing.T) {
	m := NewMock(0)
	if _, err := m.Answer(context.Background(), "  ", "ctx"); err == nil {
		t.Fatal("expected error for empty question")
	}
}

func TestMock_RespectsContext(t *testing.T) {
	m := NewMock(5 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := m.Summarize(ctx, "текст")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
