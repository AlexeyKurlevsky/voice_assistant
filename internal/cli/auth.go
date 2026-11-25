package cli

import (
	"context"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
)

type ctxKeyUser struct{}

// withUser кладёт аутентифицированного пользователя в контекст.
func withUser(ctx context.Context, u *domain.User) context.Context {
	return context.WithValue(ctx, ctxKeyUser{}, u)
}

// mustUser достаёт пользователя из контекста.
// Паникует, если нет — это означает, что auth-middleware не отработал
// (баг в коде, а не пользовательская ошибка).
func mustUser(ctx context.Context) *domain.User {
	u, ok := ctx.Value(ctxKeyUser{}).(*domain.User)
	if !ok || u == nil {
		panic("cli: user not in context — auth middleware bug")
	}
	return u
}
