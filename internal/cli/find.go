package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// newFindCmd — команда find <keyword>: полнотекстовый поиск.
// Несколько слов объединяются в один запрос (websearch_to_tsquery).
func newFindCmd(deps *Deps, userID *string, jsonOut *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "find <keyword>",
		Short: "Поиск по встречам пользователя",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			u := mustUser(ctx)

			query := strings.Join(args, " ")

			hits, err := deps.MeetingService.Search(ctx, u.ID, query)
			if err != nil {
				return err
			}

			if *jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(hits)
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "ID\tCREATED\tSTATUS\tSNIPPET")
			for _, h := range hits {
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					h.ID,
					h.CreatedAt.Format("2006-01-02 15:04:05"),
					h.Status,
					h.Snippet)
			}
			return w.Flush()
		},
	}
}
