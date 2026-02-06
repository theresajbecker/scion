/*
Copyright 2025 The Scion Authors.
*/

package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ptone/scion-agent/pkg/sciontool/hooks"
	"github.com/ptone/scion-agent/pkg/sciontool/hooks/dialects"
	"github.com/ptone/scion-agent/pkg/sciontool/hooks/handlers"
	"github.com/ptone/scion-agent/pkg/sciontool/log"
	"github.com/ptone/scion-agent/pkg/sciontool/telemetry"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/trace"
)

var (
	hookDialect string
	hookData    string
)

// hookCmd represents the hook command
var hookCmd = &cobra.Command{
	Use:   "hook [event]",
	Short: "Process harness hook events",
	Long: `The hook command processes events from agent harnesses (Claude Code, Gemini CLI).

It normalizes events from different harness formats (dialects) and updates agent
status, logs events, and performs other hook-related actions.

Events are received via stdin as JSON data. The --dialect flag specifies which
harness format to use for parsing.

Examples:
  # Process a Claude Code event from stdin
  echo '{"hook_event_name": "PreToolUse", "tool_name": "Bash"}' | sciontool hook --dialect=claude

  # Process a Gemini CLI event
  echo '{"hook_event_name": "BeforeTool", "tool_name": "shell"}' | sciontool hook --dialect=gemini

  # Use the ask_user subcommand
  sciontool hook ask_user "What should I do next?"

  # Use the task_completed subcommand
  sciontool hook task_completed "Implemented feature X"`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 0 {
			// Handle subcommands: ask_user, task_completed
			switch args[0] {
			case "ask_user":
				message := "Input requested"
				if hookData != "" {
					message = hookData
				}
				runAskUser(message)
				return
			case "task_completed":
				message := "Task completed"
				if hookData != "" {
					message = hookData
				}
				runTaskCompleted(message)
				return
			default:
				// Treat as event name (for legacy compatibility)
				runHookWithEvent(args[0])
				return
			}
		}

		// Default: process JSON from stdin
		if err := runHookFromStdin(); err != nil {
			log.Error("Hook processing failed: %v", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(hookCmd)

	hookCmd.Flags().StringVar(&hookDialect, "dialect", "claude",
		"Harness dialect for event parsing (claude, gemini)")
	hookCmd.Flags().StringVar(&hookData, "data", "",
		"Additional data for subcommands")

	// Add subcommands for direct invocation
	hookCmd.AddCommand(askUserCmd)
	hookCmd.AddCommand(taskCompletedCmd)
}

// askUserCmd represents the ask_user subcommand
var askUserCmd = &cobra.Command{
	Use:   "ask_user [message]",
	Short: "Signal that the agent is waiting for user input",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		message := "Input requested"
		if len(args) > 0 {
			message = args[0]
		}
		runAskUser(message)
	},
}

// taskCompletedCmd represents the task_completed subcommand
var taskCompletedCmd = &cobra.Command{
	Use:   "task_completed [message]",
	Short: "Signal that the agent has completed its task",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		message := "Task completed"
		if len(args) > 0 {
			message = args[0]
		}
		runTaskCompleted(message)
	},
}

// runHookFromStdin processes hook events from stdin.
func runHookFromStdin() error {
	// Check if stdin has data
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		// Stdin is a terminal, no data to process
		return nil
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}

	if len(data) == 0 {
		return nil
	}

	return processHookData(data)
}

// runHookWithEvent creates and processes a synthetic event.
func runHookWithEvent(eventName string) {
	data := map[string]interface{}{
		"hook_event_name": eventName,
	}
	jsonData, _ := json.Marshal(data)
	if err := processHookData(jsonData); err != nil {
		log.Error("Hook processing failed: %v", err)
		os.Exit(1)
	}
}

// processHookData parses and handles hook event data.
func processHookData(data []byte) error {
	var rawData map[string]interface{}
	if err := json.Unmarshal(data, &rawData); err != nil {
		return fmt.Errorf("parsing JSON: %w", err)
	}

	// Create processor with handlers
	processor := hooks.NewHarnessProcessor()

	// Register dialects
	processor.RegisterDialect(dialects.NewClaudeDialect())
	processor.RegisterDialect(dialects.NewGeminiDialect())

	// Register handlers
	statusHandler := handlers.NewStatusHandler()
	loggingHandler := handlers.NewLoggingHandler()
	promptHandler := handlers.NewPromptHandler()
	hubHandler := handlers.NewHubHandler()

	processor.AddHandler(statusHandler.Handle)
	processor.AddHandler(loggingHandler.Handle)
	processor.AddHandler(promptHandler.Handle)

	// Add Hub handler if configured
	if hubHandler != nil {
		processor.AddHandler(hubHandler.Handle)
	}

	// Add telemetry handler if telemetry is enabled
	cfg := telemetry.LoadConfig()
	if cfg != nil && cfg.Enabled {
		redactor := telemetry.NewRedactor(cfg.Redaction)

		// Create real providers for span + log export (sync mode for short-lived hook)
		ctx := context.Background()
		providers, err := telemetry.NewProviders(ctx, cfg, false)
		if err != nil {
			log.Error("Failed to create telemetry providers: %v", err)
		}
		if providers != nil {
			defer func() {
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := providers.Shutdown(shutdownCtx); err != nil {
					log.Error("Failed to shutdown telemetry providers: %v", err)
				}
			}()
		}

		var tp trace.TracerProvider
		var lp otellog.LoggerProvider
		if providers != nil {
			tp = providers.TracerProvider
			lp = providers.LoggerProvider
		}

		telemetryHandler := handlers.NewTelemetryHandler(tp, lp, redactor)
		processor.AddHandler(telemetryHandler.Handle)
	}

	return processor.ProcessRaw(rawData, hookDialect)
}

// runAskUser updates status to waiting for input.
func runAskUser(message string) {
	statusHandler := handlers.NewStatusHandler()
	loggingHandler := handlers.NewLoggingHandler()
	hubHandler := handlers.NewHubHandler()

	// Update session status to waiting for input
	if err := statusHandler.UpdateStatus(hooks.StateWaitingForInput, true); err != nil {
		log.Error("Failed to update status: %v", err)
	}

	// Log the event
	logMessage := fmt.Sprintf("Agent requested input: %s", message)
	if err := loggingHandler.LogEvent(hooks.StateWaitingForInput, logMessage); err != nil {
		log.Error("Failed to log event: %v", err)
	}

	// Send status to Hub
	if hubHandler != nil {
		if err := hubHandler.ReportWaitingForInput(message); err != nil {
			log.Error("Failed to report to Hub: %v", err)
		}
	}

	fmt.Printf("Agent asked: %s\n", message)
}

// runTaskCompleted updates status to completed.
func runTaskCompleted(message string) {
	statusHandler := handlers.NewStatusHandler()
	loggingHandler := handlers.NewLoggingHandler()
	hubHandler := handlers.NewHubHandler()

	// Update session status to completed
	if err := statusHandler.UpdateStatus(hooks.StateCompleted, true); err != nil {
		log.Error("Failed to update status: %v", err)
	}

	// Log the event
	logMessage := fmt.Sprintf("Agent completed task: %s", message)
	if err := loggingHandler.LogEvent(hooks.StateCompleted, logMessage); err != nil {
		log.Error("Failed to log event: %v", err)
	}

	// Send status to Hub
	if hubHandler != nil {
		if err := hubHandler.ReportTaskCompleted(message); err != nil {
			log.Error("Failed to report to Hub: %v", err)
		}
	}

	fmt.Printf("Agent completed: %s\n", message)
}
