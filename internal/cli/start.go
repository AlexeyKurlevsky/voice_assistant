package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func newStartCmd(deps *Deps, userID *string, jsonOut *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Зарегистрировать пользователя (идемпотентно)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			u, err := deps.UserService.EnsureExists(ctx, *userID)
			if err != nil {
				return err
			}

			if *jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
					UserID     string    `json:"user_id"`
					ExternalID string    `json:"external_id"`
					CreatedAt  time.Time `json:"created_at"`
				}{u.ID.String(), u.ExternalID, u.CreatedAt})
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"user_id: %s\nexternal_id: %s\ncreated_at: %s\n",
				u.ID, u.ExternalID, u.CreatedAt.Format(time.RFC3339))
			return nil
		},
	}
}
