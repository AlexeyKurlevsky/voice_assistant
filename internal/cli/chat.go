package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
)

func newChatCmd(deps *Deps, token *string, jsonOut *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "chat <id> <text>",
		Short: "Задать вопрос по материалам встречи",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			u := mustUser(ctx)

			meetingID, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("%w: invalid meeting id: %s", domain.ErrInvalidArgument, args[0])
			}

			question := strings.Join(args[1:], " ")

			qa, err := deps.ChatService.Ask(ctx, u.ID, meetingID, question)
			if err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					return fmt.Errorf("meeting %s not found", meetingID)
				}
				return err
			}

			if *jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
					MeetingID string `json:"meeting_id"`
					Question  string `json:"question"`
					Answer    string `json:"answer"`
				}{meetingID.String(), qa.Question, qa.Answer})
			}

			w := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(w, "Q: %s\n", qa.Question)
			_, _ = fmt.Fprintf(w, "A: %s\n", qa.Answer)
			return nil
		},
	}
}
