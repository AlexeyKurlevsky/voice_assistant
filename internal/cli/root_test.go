package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/application"
	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
)

type fakeUserRepo struct {
	user *domain.User
}

func (f *fakeUserRepo) Create(_ context.Context, externalID, _ string) (*domain.User, error) {
	if f.user == nil {
		f.user = &domain.User{ID: uuid.New(), ExternalID: externalID}
	}
	return f.user, nil
}

func (f *fakeUserRepo) GetByExternalID(_ context.Context, _ string) (*domain.User, error) {
	return f.user, nil
}

func (f *fakeUserRepo) GetByTokenHash(_ context.Context, _ string) (*domain.User, error) {
	return f.user, nil
}

func (f *fakeUserRepo) UpdateTokenHash(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}

func newTestRoot() (*cobra.Command, *bytes.Buffer) {
	out := &bytes.Buffer{}
	deps := &Deps{
		UserService: application.NewUserService(&fakeUserRepo{}),
	}
	root := NewRoot(deps, "")
	root.SetOut(out)
	root.SetErr(io.Discard)
	return root, out
}

func TestStart_Success(t *testing.T) {
	root, out := newTestRoot()
	root.SetArgs([]string{"start", "--user", "alice"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), "token:") {
		t.Errorf("start output missing token: %q", out.String())
	}
	if !strings.Contains(out.String(), "alice") {
		t.Errorf("start output missing external_id: %q", out.String())
	}
}

func TestStart_RequiresUser(t *testing.T) {
	root, _ := newTestRoot()
	root.SetArgs([]string{"start"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error without --user")
	}
}

func TestLoad_RequiresToken(t *testing.T) {
	root, _ := newTestRoot()
	root.SetArgs([]string{"load", "meeting.txt"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnknownCommand(t *testing.T) {
	root, _ := newTestRoot()
	root.SetArgs([]string{"bogus"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error for unknown command")
	}
}
