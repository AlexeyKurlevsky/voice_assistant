package application

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
)

type fakeUserRepo struct {
	created          *domain.User
	createdTokenHash string
	createErr        error

	byExternal *domain.User
	byToken    *domain.User

	updatedID        uuid.UUID
	updatedTokenHash string
}

func (f *fakeUserRepo) Create(_ context.Context, externalID, tokenHash string) (*domain.User, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = &domain.User{ID: uuid.New(), ExternalID: externalID}
	f.createdTokenHash = tokenHash
	return f.created, nil
}

func (f *fakeUserRepo) GetByExternalID(_ context.Context, _ string) (*domain.User, error) {
	if f.byExternal == nil {
		return nil, domain.ErrNotFound
	}
	return f.byExternal, nil
}

func (f *fakeUserRepo) GetByTokenHash(_ context.Context, _ string) (*domain.User, error) {
	if f.byToken == nil {
		return nil, domain.ErrNotFound
	}
	return f.byToken, nil
}

func (f *fakeUserRepo) UpdateTokenHash(_ context.Context, id uuid.UUID, tokenHash string) error {
	f.updatedID = id
	f.updatedTokenHash = tokenHash
	return nil
}

func TestUserService_Register(t *testing.T) {
	repo := &fakeUserRepo{}
	svc := NewUserService(repo)

	u, token, err := svc.Register(context.Background(), "alice")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if u.ExternalID != "alice" {
		t.Errorf("ExternalID = %q", u.ExternalID)
	}
	if len(token) != 64 {
		t.Fatalf("token length = %d, want 64", len(token))
	}
	if _, err := hex.DecodeString(token); err != nil {
		t.Fatalf("token is not hex: %v", err)
	}
	if repo.createdTokenHash == "" || len(repo.createdTokenHash) != 64 {
		t.Fatalf("stored token hash = %q", repo.createdTokenHash)
	}
	if repo.createdTokenHash == token {
		t.Error("plaintext token must not be stored")
	}
}

func TestUserService_Register_InvalidExternalID(t *testing.T) {
	svc := NewUserService(&fakeUserRepo{})
	for _, id := range []string{"", "bad id!", string(make([]byte, 65))} {
		if _, _, err := svc.Register(context.Background(), id); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Errorf("external_id %q: err = %v", id, err)
		}
	}
}

func TestUserService_Register_AlreadyExists(t *testing.T) {
	svc := NewUserService(&fakeUserRepo{createErr: domain.ErrAlreadyExists})
	_, _, err := svc.Register(context.Background(), "alice")
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("err = %v, want ErrAlreadyExists", err)
	}
}

func TestUserService_Authenticate(t *testing.T) {
	user := &domain.User{ID: uuid.New(), ExternalID: "alice"}
	svc := NewUserService(&fakeUserRepo{byToken: user})

	got, err := svc.Authenticate(context.Background(), "some-token")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("user id = %s", got.ID)
	}
}

func TestUserService_Authenticate_EmptyToken(t *testing.T) {
	svc := NewUserService(&fakeUserRepo{})
	if _, err := svc.Authenticate(context.Background(), ""); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("err = %v", err)
	}
}

func TestUserService_Authenticate_NotFound(t *testing.T) {
	svc := NewUserService(&fakeUserRepo{})
	if _, err := svc.Authenticate(context.Background(), "unknown"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestUserService_Rotate(t *testing.T) {
	repo := &fakeUserRepo{}
	svc := NewUserService(repo)
	id := uuid.New()

	token, err := svc.Rotate(context.Background(), id)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if len(token) != 64 {
		t.Fatalf("token length = %d", len(token))
	}
	if repo.updatedID != id {
		t.Errorf("updatedID = %s, want %s", repo.updatedID, id)
	}
	if repo.updatedTokenHash == "" || repo.updatedTokenHash == token {
		t.Error("token hash must be stored, not plaintext")
	}
}
