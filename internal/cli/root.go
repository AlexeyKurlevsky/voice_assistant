package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/application"
	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
)

// Deps — зависимости CLI-команд (сервисы из application).
type Deps struct {
	UserService    *application.UserService
	MeetingService *application.MeetingService
	ChatService    *application.ChatService
}

// NewRoot собирает корневую команду и все подкоманды.
//
// Аутентификация — в PersistentPreRunE:
//   - start: требует --user, токен не нужен;
//   - все остальные: требуют --token (или MS_TOKEN / файл), кладут
//     пользователя в context; в RunE достаётся через mustUser(ctx).
func NewRoot(deps *Deps, defaultToken string) *cobra.Command {
	var (
		userID  string
		token   string
		jsonOut bool
	)

	root := &cobra.Command{
		Use:           "voice-assistant",
		Short:         "Voice assistant: загрузка, обработка и поиск встреч",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Bare invocation (voice-assistant без подкоманды) — cobra
			// покажет help; auth не нужен.
			if cmd.Parent() == nil {
				return nil
			}

			name := cmd.Name()
			if name == "help" || name == "completion" {
				return nil
			}

			if name == "start" {
				if strings.TrimSpace(userID) == "" {
					return fmt.Errorf("%w: --user is required", domain.ErrInvalidArgument)
				}
				return nil
			}

			if strings.TrimSpace(token) == "" {
				return fmt.Errorf("%w: --token is required (or set MS_TOKEN / ~/.voice-assistant-token)", domain.ErrInvalidArgument)
			}

			u, err := deps.UserService.Authenticate(cmd.Context(), token)
			if err != nil {
				return err
			}
			cmd.SetContext(withUser(cmd.Context(), u))
			return nil
		},
	}

	root.PersistentFlags().StringVar(&userID, "user", "", "external_id (только для start)")
	root.PersistentFlags().StringVar(&token, "token", defaultToken,
		"session-токен (или MS_TOKEN / ~/.voice-assistant-token)")
	root.PersistentFlags().BoolVar(&jsonOut, "json", false, "вывод в JSON")

	root.AddCommand(
		newStartCmd(deps, &userID, &jsonOut),
		newRotateCmd(deps, &jsonOut),
		newLoadCmd(deps, &userID, &jsonOut),
		newListCmd(deps, &userID, &jsonOut),
		newStatusCmd(deps, &userID, &jsonOut),
		newGetCmd(deps, &userID, &jsonOut),
		newFindCmd(deps, &userID, &jsonOut),
		newChatCmd(deps, &userID, &jsonOut),
	)

	return root
}
