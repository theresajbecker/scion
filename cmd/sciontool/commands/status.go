/*
Copyright 2025 The Scion Authors.
*/

package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ptone/scion-agent/pkg/sciontool/hooks"
	"github.com/ptone/scion-agent/pkg/sciontool/hooks/handlers"
	"github.com/ptone/scion-agent/pkg/sciontool/hub"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status <status-type> <message>",
	Short: "Update agent status",
	Long: `The status command updates the agent's session status and logs the event.

This is used by agents to signal state changes to the scion orchestrator.

Status Types:
  ask_user        Signal that the agent is waiting for user input
  task_completed  Signal that the agent has completed its task

Examples:
  # Signal waiting for user input
  sciontool status ask_user "What should I do next?"

  # Signal task completion
  sciontool status task_completed "Implemented feature X"`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		statusType := args[0]
		message := strings.Join(args[1:], " ")

		switch statusType {
		case "ask_user":
			if message == "" {
				message = "Input requested"
			}
			runStatusAskUser(message)
		case "task_completed":
			if message == "" {
				message = "Task completed"
			}
			runStatusTaskCompleted(message)
		default:
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: unknown status type %q\n", statusType)
			fmt.Fprintf(cmd.ErrOrStderr(), "Valid types: ask_user, task_completed\n")
			cmd.Root().SetArgs([]string{"status", "--help"})
			cmd.Root().Execute()
		}
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

// runStatusAskUser updates status to waiting for input.
func runStatusAskUser(message string) {
	statusHandler := handlers.NewStatusHandler()
	loggingHandler := handlers.NewLoggingHandler()

	// Update session status to waiting for input
	if err := statusHandler.UpdateStatus(hooks.StateWaitingForInput, true); err != nil {
		logError("Failed to update status: %v", err)
	}

	// Log the event
	logMessage := fmt.Sprintf("Agent requested input: %s", message)
	if err := loggingHandler.LogEvent(hooks.StateWaitingForInput, logMessage); err != nil {
		logError("Failed to log event: %v", err)
	}

	// Report to Hub if in hosted mode
	if hubClient := hub.NewClient(); hubClient != nil && hubClient.IsConfigured() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := hubClient.ReportIdle(ctx, message); err != nil {
			logError("Failed to report to Hub: %v", err)
		}
	}

	fmt.Printf("Agent asked: %s\n", message)
}

// runStatusTaskCompleted updates status to completed.
func runStatusTaskCompleted(message string) {
	statusHandler := handlers.NewStatusHandler()
	loggingHandler := handlers.NewLoggingHandler()

	// Update session status to completed
	if err := statusHandler.UpdateStatus(hooks.StateCompleted, true); err != nil {
		logError("Failed to update status: %v", err)
	}

	// Log the event
	logMessage := fmt.Sprintf("Agent completed task: %s", message)
	if err := loggingHandler.LogEvent(hooks.StateCompleted, logMessage); err != nil {
		logError("Failed to log event: %v", err)
	}

	// Report to Hub if in hosted mode
	if hubClient := hub.NewClient(); hubClient != nil && hubClient.IsConfigured() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := hubClient.ReportTaskCompleted(ctx, message); err != nil {
			logError("Failed to report to Hub: %v", err)
		}
	}

	fmt.Printf("Agent completed: %s\n", message)
}
