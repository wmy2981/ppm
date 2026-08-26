package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wmy2981/ppm/internal/elevate"
	"github.com/wmy2981/ppm/internal/store"
	"github.com/wmy2981/ppm/internal/ui"
)

// version is injected at build time from the repo root VERSION file via
// -ldflags "-X main.version=$(cat VERSION)".
var version = "dev"

func main() {
	elevate.ElevateOrExit()

	st, err := store.Open()
	if err != nil {
		elevate.Fatal("open data dir: %v", err)
	}
	notes, err := st.LoadNotes()
	if err != nil {
		// notes are auxiliary; warn but continue with empty
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		notes = map[string]string{}
	}

	p := tea.NewProgram(ui.New(version, st, notes), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		elevate.Fatal("tui: %v", err)
	}
}
