package cli

import (
	"github.com/spf13/cobra"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/application"
)

type Deps struct {
	UserService    *application.UserService
	MeetingService *application.MeetingService
}

func NewRoot(deps *Deps) *cobra.Command {
	var (
		userID  string
		jsonOut bool
	)

	root := &cobra.Command{
		Use:           "voice-assistant",
		Short:         "Voice assistant: загрузка, обработка и поиск встреч",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&userID, "user", "", "external_id пользователя (обязательно)")
	root.PersistentFlags().BoolVar(&jsonOut, "json", false, "вывод в JSON")
	_ = root.MarkPersistentFlagRequired("user")

	root.AddCommand(
		newStartCmd(deps, &userID, &jsonOut),
		newLoadCmd(deps, &userID, &jsonOut),
		newListCmd(deps, &userID, &jsonOut),
	)

	return root
}
