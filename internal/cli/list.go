package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newListCmd(deps *Deps, userID *string, jsonOut *bool) *cobra.Command {
	var limit, offset int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Список встреч пользователя",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			u, err := deps.UserService.EnsureExists(ctx, *userID)
			if err != nil {
				return err
			}

			items, err := deps.MeetingService.List(ctx, u.ID, limit, offset)
			if err != nil {
				return err
			}

			if *jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(items)
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "ID\tCREATED\tSTATUS\tSUMMARY")
			for _, it := range items {
				summary := it.Summary
				if summary == "" {
					summary = "—"
				} else if len(summary) > 60 {
					summary = summary[:60] + "..."
				}
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					it.ID,
					it.CreatedAt.Format("2006-01-02 15:04:05"),
					it.Status,
					summary)
			}
			return w.Flush()
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 50, "максимум записей (1..500)")
	cmd.Flags().IntVar(&offset, "offset", 0, "смещение")
	return cmd
}
