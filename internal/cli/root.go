// Package cli — презентационный слой: cobra-команды, парсинг аргументов,
// форматирование вывода. Бизнес-логика — в application.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/application"
)

// Deps — зависимости CLI-команд (сервисы из application).
type Deps struct {
	UserService    *application.UserService
	MeetingService *application.MeetingService
	ChatService    *application.ChatService
}

// NewRoot собирает корневую команду и все подкоманды.
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
		newStatusCmd(deps, &userID, &jsonOut),
		newGetCmd(deps, &userID, &jsonOut),
		newFindCmd(deps, &userID, &jsonOut),
		newChatCmd(deps, &userID, &jsonOut),
	)

	return root
}
