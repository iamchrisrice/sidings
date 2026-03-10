package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func completionCmd(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion",
		Short: "Generate shell completion scripts",
		Long:  "Generate shell completion scripts for bash, zsh, or fish.",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "bash",
			Short: "Print bash completion script",
			RunE: func(cmd *cobra.Command, args []string) error {
				return root.GenBashCompletion(os.Stdout)
			},
		},
		&cobra.Command{
			Use:   "zsh",
			Short: "Print zsh completion script",
			RunE: func(cmd *cobra.Command, args []string) error {
				return root.GenZshCompletion(os.Stdout)
			},
		},
		&cobra.Command{
			Use:   "fish",
			Short: "Print fish completion script",
			RunE: func(cmd *cobra.Command, args []string) error {
				return root.GenFishCompletion(os.Stdout, true)
			},
		},
		&cobra.Command{
			Use:   "install",
			Short: "Detect shell and install completion automatically",
			RunE: func(cmd *cobra.Command, args []string) error {
				return installCompletion(root)
			},
		},
	)

	return cmd
}

func installCompletion(root *cobra.Command) error {
	shell := filepath.Base(os.Getenv("SHELL"))
	switch shell {
	case "bash":
		return installBash(root)
	case "zsh":
		return installZsh(root)
	case "fish":
		return installFish(root)
	default:
		return fmt.Errorf("unsupported shell %q — run 'sidings completion bash|zsh|fish' and source manually", shell)
	}
}

func installBash(root *cobra.Command) error {
	dir := filepath.Join(os.Getenv("HOME"), ".bash_completion.d")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	path := filepath.Join(dir, "sidings")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer f.Close()
	if err := root.GenBashCompletion(f); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "✓ completion installed for bash")
	fmt.Fprintf(os.Stderr, "  Add to ~/.bashrc: source %s\n", path)
	return nil
}

func installZsh(root *cobra.Command) error {
	// Prefer ~/.zfunc if it already exists (user has set it up), otherwise use ~/.zsh/completions.
	dir := filepath.Join(os.Getenv("HOME"), ".zsh", "completions")
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".zfunc")); err == nil {
		dir = filepath.Join(os.Getenv("HOME"), ".zfunc")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	path := filepath.Join(dir, "_sidings")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer f.Close()
	if err := root.GenZshCompletion(f); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "✓ completion installed for zsh")
	fmt.Fprintf(os.Stderr, "  Add to ~/.zshrc: fpath=(%s $fpath) && autoload -U compinit && compinit\n", dir)
	return nil
}

func installFish(root *cobra.Command) error {
	dir := filepath.Join(os.Getenv("HOME"), ".config", "fish", "completions")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	path := filepath.Join(dir, "sidings.fish")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer f.Close()
	if err := root.GenFishCompletion(f, true); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "✓ completion installed for fish")
	return nil
}
