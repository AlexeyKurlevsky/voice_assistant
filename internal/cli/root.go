package cli

import (
	"github.com/spf13/cobra"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/application"
)

// Deps — то, что нужно CLI-командам.
// По мере роста команд добавятся другие сервисы и пул воркеров.
type Deps struct {
	UserService *application.UserService
}

// NewRoot создаёт корневую команду со всеми подкомандами.
func NewRoot(deps *Deps) *cobra.Command {
	var (
		userID  string
		jsonOut bool
	)

	root := &cobra.Command{
		Use:           "meeting-assistant",
		Short:         "Meeting assistant: загрузка, обработка и поиск встреч",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&userID, "user", "", "external_id пользователя (обязательно)")
	root.PersistentFlags().BoolVar(&jsonOut, "json", false, "вывод в JSON")
	_ = root.MarkPersistentFlagRequired("user")

	root.AddCommand(
		newStartCmd(deps, &userID, &jsonOut),
	)

	return root
}

// extractUser — единый хелпер: возвращает user_id, читает флаг из родителя.
// Пока используется в start.go, позже — во всех командах.
func extractUser(cmd *cobra.Command) (string, error) {
	id, err := cmd.Flags().GetString("user")
	if err != nil {
		return "", err
	}
	return id, nil
}
