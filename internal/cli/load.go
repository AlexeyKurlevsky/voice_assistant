package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// newLoadCmd — команда load <path>: загрузка файла и постановка в очередь.
func newLoadCmd(deps *Deps, userID *string, jsonOut *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "load <path>",
		Short: "Загрузить файл встречи и поставить в очередь",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			u := mustUser(ctx)

			m, t, err := deps.MeetingService.Load(ctx, u.ID, args[0])
			if err != nil {
				return err
			}
			if *jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
					MeetingID string `json:"meeting_id"`
					Status    string `json:"status"`
				}{m.ID.String(), string(t.Status)})
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"meeting_id: %s\nstatus: %s\n", m.ID, t.Status)
			return nil
		},
	}
}
