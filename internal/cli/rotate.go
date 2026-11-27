package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// newRotateCmd — команда rotate: перевыпуск токена.
// Требует действующий --token (проверяется в middleware).
func newRotateCmd(deps *Deps, jsonOut *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "rotate",
		Short: "Перевыпустить токен (старый инвалидируется)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			u := mustUser(ctx)

			newToken, err := deps.UserService.Rotate(ctx, u.ID)
			if err != nil {
				return err
			}

			if *jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
					UserID     string `json:"user_id"`
					ExternalID string `json:"external_id"`
					Token      string `json:"token"`
				}{u.ID.String(), u.ExternalID, newToken})
			}

			w := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(w, "external_id: %s\n", u.ExternalID)
			_, _ = fmt.Fprintf(w, "\nnew token: %s\n\n", newToken)
			_, _ = fmt.Fprintln(w, "Старый токен инвалидирован. Сохраните новый.")
			return nil
		},
	}
}
