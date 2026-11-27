package domain

import "testing"

func TestStatus_IsValid(t *testing.T) {
	for _, s := range []Status{
		StatusCreated, StatusProcessing, StatusTranscribed,
		StatusSummarized, StatusCompleted, StatusFailed,
	} {
		if !s.IsValid() {
			t.Errorf("status %q should be valid", s)
		}
	}
	if Status("bogus").IsValid() {
		t.Error("unexpected status should be invalid")
	}
	if Status("").IsValid() {
		t.Error("empty status should be invalid")
	}
}

func TestStatus_IsTerminal(t *testing.T) {
	if !StatusCompleted.IsTerminal() {
		t.Error("completed should be terminal")
	}
	if !StatusFailed.IsTerminal() {
		t.Error("failed should be terminal")
	}
	for _, s := range []Status{
		StatusCreated, StatusProcessing, StatusTranscribed, StatusSummarized,
	} {
		if s.IsTerminal() {
			t.Errorf("status %q should not be terminal", s)
		}
	}
}
