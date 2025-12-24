package main

import (
	"claude-squad/app"
	cmd2 "claude-squad/cmd"
	"claude-squad/config"
	"claude-squad/daemon"
	"claude-squad/log"
	"claude-squad/session"
	"claude-squad/session/git"
	"claude-squad/session/tmux"
	"claude-squad/session/zellij"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	version         = "1.0.13"
	programFlag     string
	autoYesFlag     bool
	daemonFlag      bool
	multiplexerFlag string
	rootCmd     = &cobra.Command{
		Use:   "claude-squad",
		Short: "Claude Squad - Manage multiple AI agents like Claude Code, Aider, Codex, and Amp.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			log.Initialize(daemonFlag)
			defer log.Close()

			if daemonFlag {
				cfg := config.LoadConfig()
				err := daemon.RunDaemon(cfg)
				log.ErrorLog.Printf("failed to start daemon %v", err)
				return err
			}

			cfg := config.LoadConfig()

			// Program flag overrides config
			program := cfg.DefaultProgram
			if programFlag != "" {
				program = programFlag
			}
			// AutoYes flag overrides config
			autoYes := cfg.AutoYes
			if autoYesFlag {
				autoYes = true
			}
			// Multiplexer flag overrides config
			if multiplexerFlag != "" {
				if multiplexerFlag != "tmux" && multiplexerFlag != "zellij" {
					return fmt.Errorf("invalid multiplexer: %s (must be 'tmux' or 'zellij')", multiplexerFlag)
				}
				// Zellij is not supported on Windows
				if multiplexerFlag == "zellij" && runtime.GOOS == "windows" {
					return fmt.Errorf("zellij is not supported on Windows, use tmux instead")
				}
				cfg.Multiplexer = multiplexerFlag
			}
			if autoYes {
				defer func() {
					if err := daemon.LaunchDaemon(); err != nil {
						log.ErrorLog.Printf("failed to launch daemon: %v", err)
					}
				}()
			}
			// Kill any daemon that's running.
			if err := daemon.StopDaemon(); err != nil {
				log.ErrorLog.Printf("failed to stop daemon: %v", err)
			}

			return app.Run(ctx, program, autoYes)
		},
	}

	resetCmd = &cobra.Command{
		Use:   "reset",
		Short: "Reset all stored instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			log.Initialize(false)
			defer log.Close()

			state := config.LoadState()
			storage, err := session.NewStorage(state)
			if err != nil {
				return fmt.Errorf("failed to initialize storage: %w", err)
			}
			if err := storage.DeleteAllInstances(); err != nil {
				return fmt.Errorf("failed to reset storage: %w", err)
			}
			fmt.Println("Storage has been reset successfully")

			// Clean up tmux sessions
			if err := tmux.CleanupSessions(cmd2.MakeExecutor()); err != nil {
				// Log but don't fail - tmux might not be installed
				log.WarningLog.Printf("failed to cleanup tmux sessions: %v", err)
			} else {
				fmt.Println("Tmux sessions have been cleaned up")
			}

			// Clean up zellij sessions (only on non-Windows)
			if runtime.GOOS != "windows" {
				if err := zellij.CleanupSessions(cmd2.MakeExecutor()); err != nil {
					// Log but don't fail - zellij might not be installed
					log.WarningLog.Printf("failed to cleanup zellij sessions: %v", err)
				} else {
					fmt.Println("Zellij sessions have been cleaned up")
				}
			}

			if err := git.CleanupWorktrees(); err != nil {
				return fmt.Errorf("failed to cleanup worktrees: %w", err)
			}
			fmt.Println("Worktrees have been cleaned up")

			// Kill any daemon that's running.
			if err := daemon.StopDaemon(); err != nil {
				return err
			}
			fmt.Println("daemon has been stopped")

			return nil
		},
	}

	debugCmd = &cobra.Command{
		Use:   "debug",
		Short: "Print debug information like config paths",
		RunE: func(cmd *cobra.Command, args []string) error {
			log.Initialize(false)
			defer log.Close()

			cfg := config.LoadConfig()

			configDir, err := config.GetConfigDir()
			if err != nil {
				return fmt.Errorf("failed to get config directory: %w", err)
			}
			configJson, _ := json.MarshalIndent(cfg, "", "  ")

			fmt.Printf("Config: %s\n%s\n", filepath.Join(configDir, config.ConfigFileName), configJson)

			return nil
		},
	}

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version number of claude-squad",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("claude-squad version %s\n", version)
			fmt.Printf("https://github.com/smtg-ai/claude-squad/releases/tag/v%s\n", version)
		},
	}
)

func init() {
	rootCmd.Flags().StringVarP(&programFlag, "program", "p", "",
		"Program to run in new instances (e.g. 'aider --model ollama_chat/gemma3:1b')")
	rootCmd.Flags().BoolVarP(&autoYesFlag, "autoyes", "y", false,
		"[experimental] If enabled, all instances will automatically accept prompts")
	rootCmd.Flags().BoolVar(&daemonFlag, "daemon", false, "Run a program that loads all sessions"+
		" and runs autoyes mode on them.")
	rootCmd.Flags().StringVarP(&multiplexerFlag, "multiplexer", "m", "",
		"Terminal multiplexer to use ('tmux' or 'zellij'). Defaults to zellij on Unix, tmux on Windows.")

	// Hide the daemonFlag as it's only for internal use
	err := rootCmd.Flags().MarkHidden("daemon")
	if err != nil {
		panic(err)
	}

	rootCmd.AddCommand(debugCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(resetCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
	}
}
