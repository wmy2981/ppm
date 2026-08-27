package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wmy2981/ppm/internal/cli"
	"github.com/wmy2981/ppm/internal/elevate"
	"github.com/wmy2981/ppm/internal/store"
	"github.com/wmy2981/ppm/internal/ui"
)

// version is injected at build time from the repo root VERSION file via
// -ldflags "-X main.version=$(cat VERSION)".
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		runCLI()
	} else {
		runTUI()
	}
}

func runTUI() {
	elevate.ElevateOrExit()

	st, err := store.Open()
	if err != nil {
		elevate.Fatal("open data dir: %v", err)
	}
	notes, err := st.LoadNotes()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		notes = map[string]string{}
	}

	p := tea.NewProgram(ui.New(version, st, notes), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		elevate.Fatal("tui: %v", err)
	}
}

func runCLI() {
	app := cli.App(version)
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
