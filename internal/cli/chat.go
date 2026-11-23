package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
)

// newChatCmd — команда chat <id> <text>: вопрос по материалам встречи.
// Текст может содержать пробелы без кавычек: chat <id> какие сроки обсудили?
func newChatCmd(deps *Deps, userID *string, jsonOut *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "chat <id> <text>",
		Short: "Задать вопрос по материалам встречи",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			u, err := deps.UserService.EnsureExists(ctx, *userID)
			if err != nil {
				return err
			}

			meetingID, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("%w: invalid meeting id: %s", domain.ErrInvalidArgument, args[0])
			}

			question := strings.Join(args[1:], " ")

			qa, err := deps.ChatService.Ask(ctx, u.ID, meetingID, question)
			if err != nil {
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
