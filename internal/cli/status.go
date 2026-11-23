package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
)

// newStatusCmd — команда status <id>: текущий статус обработки.
// С --verbose — история переходов; с --watch — polling до терминального статуса.
func newStatusCmd(deps *Deps, userID *string, jsonOut *bool) *cobra.Command {
	var (
		verbose  bool
		watch    bool
		interval time.Duration
	)

	cmd := &cobra.Command{
		Use:   "status <id>",
		Short: "Текущий статус обработки встречи",
		Args:  cobra.ExactArgs(1),
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

			if watch {
				return runStatusWatch(ctx, cmd, deps, meetingID, u.ID, interval, *jsonOut)
			}

			task, err := deps.MeetingService.Status(ctx, meetingID, u.ID)
			if err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					return fmt.Errorf("meeting %s not found", meetingID)
				}
				return err
			}

			var history []domain.StatusChange
			if verbose {
				history, err = deps.MeetingService.StatusHistory(ctx, task.ID)
				if err != nil {
					return err
				}
			}

			printStatus(cmd, meetingID, task, history, *jsonOut)
			return nil
		},
	}

	cmd.Flags().BoolVar(&verbose, "verbose", false, "показать историю переходов")
	cmd.Flags().BoolVar(&watch, "watch", false, "опрашивать статус до завершения")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "интервал опроса (с --watch)")
	return cmd
}

// runStatusWatch опрашивает статус с интервалом до терминального состояния.
// В text-режиме — обновление в одной строке (carriage return).
// В JSON-режиме — NDJSON: по одной строке на каждое изменение статуса.
func runStatusWatch(
	ctx context.Context,
	cmd *cobra.Command,
	deps *Deps,
	meetingID, userID uuid.UUID,
	interval time.Duration,
	jsonOut bool,
) error {
	w := cmd.OutOrStdout()
	lastStatus := ""

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		task, err := deps.MeetingService.Status(ctx, meetingID, userID)
		if err != nil {
			return err
		}

		if jsonOut {
			if string(task.Status) != lastStatus {
				_ = json.NewEncoder(w).Encode(struct {
					MeetingID    string `json:"meeting_id"`
					Status       string `json:"status"`
					UpdatedAt    string `json:"updated_at"`
					ErrorMessage string `json:"error_message,omitempty"`
				}{
					MeetingID:    meetingID.String(),
					Status:       string(task.Status),
					UpdatedAt:    task.UpdatedAt.Format(time.RFC3339),
					ErrorMessage: task.ErrorMessage,
				})
			}
		} else {
			line := fmt.Sprintf("%s  status=%s", time.Now().Format("15:04:05"), task.Status)
			if task.ErrorMessage != "" {
				line += "  error=" + task.ErrorMessage
			}
			_, _ = fmt.Fprintf(w, "\r%s", line)
		}
		lastStatus = string(task.Status)

		if task.Status.IsTerminal() {
			if !jsonOut {
				_, _ = fmt.Fprintln(w)
			}
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

// printStatus — однократный вывод (без --watch).
// Статусы в истории отрисовываются стрелками: предыдущий берётся из соседней записи.
func printStatus(cmd *cobra.Command, meetingID uuid.UUID, task *domain.Task, history []domain.StatusChange, jsonOut bool) {
	if jsonOut {
		type hEntry struct {
			CreatedAt string `json:"created_at"`
			Status    string `json:"status"`
		}
		hs := make([]hEntry, 0, len(history))
		for _, h := range history {
			hs = append(hs, hEntry{
				CreatedAt: h.CreatedAt.Format(time.RFC3339),
				Status:    string(h.Status),
			})
		}
		_ = json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
			MeetingID    string   `json:"meeting_id"`
			Status       string   `json:"status"`
			CreatedAt    string   `json:"created_at"`
			UpdatedAt    string   `json:"updated_at"`
			ErrorMessage string   `json:"error_message,omitempty"`
			History      []hEntry `json:"history,omitempty"`
		}{
			MeetingID:    meetingID.String(),
			Status:       string(task.Status),
			CreatedAt:    task.CreatedAt.Format(time.RFC3339),
			UpdatedAt:    task.UpdatedAt.Format(time.RFC3339),
			ErrorMessage: task.ErrorMessage,
			History:      hs,
		})
		return
	}

	w := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(w, "meeting_id: %s\n", meetingID)
	_, _ = fmt.Fprintf(w, "status:     %s\n", task.Status)
	_, _ = fmt.Fprintf(w, "created_at: %s\n", task.CreatedAt.Format("2006-01-02 15:04:05"))
	_, _ = fmt.Fprintf(w, "updated_at: %s\n", task.UpdatedAt.Format("2006-01-02 15:04:05"))
	if task.ErrorMessage != "" {
		_, _ = fmt.Fprintf(w, "error:      %s\n", task.ErrorMessage)
	}
	if len(history) > 0 {
		_, _ = fmt.Fprintln(w, "history:")
		prev := ""
		for _, h := range history {
			from := prev
			if from == "" {
				from = "(null)"
			}
			_, _ = fmt.Fprintf(w, "  %s  %s -> %s\n",
				h.CreatedAt.Format("2006-01-02 15:04:05"), from, h.Status)
			prev = string(h.Status)
		}
	}
}
