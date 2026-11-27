package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
)

// newGetCmd — команда get <id>: полная транскрипция встречи.
func newGetCmd(deps *Deps, userID *string, jsonOut *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Полная транскрипция встречи",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			u := mustUser(ctx)

			meetingID, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("%w: invalid meeting id: %s", domain.ErrInvalidArgument, args[0])
			}

			text, err := deps.MeetingService.Transcript(ctx, meetingID, u.ID)
			if err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					return fmt.Errorf("meeting %s not found (or transcription not ready)", meetingID)
				}
				return err
			}

			if *jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
					MeetingID     string `json:"meeting_id"`
					Transcription string `json:"transcription"`
				}{meetingID.String(), text})
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), text)
			return nil
		},
	}
}
