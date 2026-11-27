package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
)

// newStartCmd — команда start --user <external_id>: регистрация + токен.
func newStartCmd(deps *Deps, userID *string, jsonOut *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Зарегистрировать пользователя и получить токен (показывается один раз)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			if strings.TrimSpace(*userID) == "" {
				return fmt.Errorf("%w: --user is required", domain.ErrInvalidArgument)
			}

			u, token, err := deps.UserService.Register(ctx, *userID)
			if err != nil {
				if errors.Is(err, domain.ErrAlreadyExists) {
					return fmt.Errorf("user %q already exists; use `rotate` with the current token, or reset via psql", *userID)
				}
				return err
			}

			if *jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
					UserID     string    `json:"user_id"`
					ExternalID string    `json:"external_id"`
					Token      string    `json:"token"`
					CreatedAt  time.Time `json:"created_at"`
				}{u.ID.String(), u.ExternalID, token, u.CreatedAt})
			}

			w := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(w, "user_id: %s\n", u.ID)
			_, _ = fmt.Fprintf(w, "external_id: %s\n", u.ExternalID)
			_, _ = fmt.Fprintf(w, "created_at: %s\n", u.CreatedAt.Format(time.RFC3339))
			_, _ = fmt.Fprintf(w, "\ntoken: %s\n\n", token)
			_, _ = fmt.Fprintln(w, "Сохраните токен — он показывается один раз.")
			_, _ = fmt.Fprintln(w, "Дальше: --token <token> или MS_TOKEN=<token> или ~/.voice-assistant-token.")
			return nil
		},
	}
}
