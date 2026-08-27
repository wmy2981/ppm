package cli

import (
	"fmt"
	"strings"

	"github.com/urfave/cli/v2"

	"github.com/wmy2981/ppm/internal/elevate"
	"github.com/wmy2981/ppm/internal/netsh"
	"github.com/wmy2981/ppm/internal/store"
)

var version string

// App returns the urfave/cli application with all subcommands registered.
func App(v string) *cli.App {
	version = v
	return &cli.App{
		Name:    "ppm",
		Usage:   "Portproxy Manager - manage Windows netsh portproxy rules",
		Version: v,
		Commands: []*cli.Command{
			listCmd,
			addCmd,
			editCmd,
			deleteCmd,
			testCmd,
			exportCmd,
			importCmd,
			tuiCmd,
			{
				Name:  "version",
				Usage: "Print the version number",
				Action: func(c *cli.Context) error {
					fmt.Println("ppm version", version)
					return nil
				},
			},
		},
	}
}

var listCmd = &cli.Command{
	Name:  "list",
	Usage: "List all portproxy rules",
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "json", Usage: "Output as JSON"},
	},
	Action: listAction,
}

var addCmd = &cli.Command{
	Name:      "add",
	Usage:     "Add a new portproxy rule",
	ArgsUsage: "<listen> <connect> [note]",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "listen", Usage: "Listen address and port (e.g. :8080 or 192.168.1.1:8080)"},
		&cli.StringFlag{Name: "connect", Usage: "Connect address and port (e.g. 10.0.0.1:80)"},
		&cli.StringFlag{Name: "note", Usage: "Optional note for this rule"},
		&cli.BoolFlag{Name: "elevate", Usage: "Request admin privileges via UAC before executing"},
	},
	Action: addAction,
}

var editCmd = &cli.Command{
	Name:      "edit",
	Usage:     "Edit an existing portproxy rule (delete + recreate)",
	ArgsUsage: "<originlisten> [<listen>] [<connect>] [note]",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "connect", Usage: "New connect address and port"},
		&cli.StringFlag{Name: "note", Usage: "New note"},
		&cli.StringFlag{Name: "listen", Usage: "New listen address and port"},
		&cli.BoolFlag{Name: "elevate", Usage: "Request admin privileges via UAC before executing"},
	},
	Action: editAction,
}

var deleteCmd = &cli.Command{
	Name:  "delete",
	Usage: "Delete a portproxy rule",
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "elevate", Usage: "Request admin privileges via UAC before executing"},
	},
	Action: deleteAction,
}

var testCmd = &cli.Command{
	Name:  "test",
	Usage: "Test connectivity to a rule's target",
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "all", Usage: "Test all rules"},
		&cli.BoolFlag{Name: "json", Usage: "Output as JSON"},
	},
	Action: testAction,
}

var exportCmd = &cli.Command{
	Name:  "export",
	Usage: "Export all rules and notes to a backup file",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "output", Aliases: []string{"o"}, Usage: "Output file path (default: auto-generated in data dir)"},
	},
	Action: exportAction,
}

var importCmd = &cli.Command{
	Name:  "import",
	Usage: "Import rules from a backup file",
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "elevate", Usage: "Request admin privileges via UAC before executing"},
	},
	Action: importAction,
}

var tuiCmd = &cli.Command{
	Name:   "tui",
	Usage:  "Launch the interactive TUI",
	Action: tuiAction,
}

// parseListenArg parses a positional listen argument into addr and port.
// Accepts ":8080" (default 0.0.0.0), "0.0.0.0:8080", or "192.168.1.1:8080".
func parseListenArg(s string) (addr, port string, err error) {
	idx := strings.LastIndex(s, ":")
	if idx < 0 {
		return "", "", fmt.Errorf("invalid listen address %q: expected format :port or ip:port", s)
	}
	addr = s[:idx]
	port = s[idx+1:]
	if addr == "" {
		addr = "0.0.0.0"
	}
	if !netsh.IsPort(port) {
		return "", "", fmt.Errorf("invalid port %q", port)
	}
	return addr, port, nil
}

// findRule searches rules for one matching addr:port.
func findRule(rules []netsh.Rule, addr, port string) *netsh.Rule {
	for i := range rules {
		if rules[i].ListenAddr == addr && rules[i].ListenPort == port {
			return &rules[i]
		}
	}
	return nil
}

// openStore opens the data store, loads notes, and returns all current rules.
func openStore() (*store.Store, map[string]string, []netsh.Rule, error) {
	st, err := store.Open()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open data dir: %w", err)
	}
	notes, err := st.LoadNotes()
	if err != nil {
		notes = map[string]string{}
	}
	rules, err := netsh.ListRules()
	if err != nil {
		return st, notes, nil, fmt.Errorf("list rules: %w", err)
	}
	return st, notes, rules, nil
}

func requireElevate(c *cli.Context, extra ...map[string]string) {
	if c.Bool("elevate") {
		elevate.ElevateOrExit()
		return
	}
	for _, m := range extra {
		if m["elevate"] == "true" {
			elevate.ElevateOrExit()
			return
		}
	}
}

// leftoverFlags extracts --key value pairs that urfave/cli missed because they
// appeared after positional arguments. Returns only the positionals (non-flag
// args) and a map of extracted flag values. Boolean flags (no value) are set to
// "true".
func leftoverFlags(args []string, flagNames ...string) (positionals []string, flags map[string]string) {
	known := make(map[string]bool, len(flagNames))
	for _, n := range flagNames {
		known[n] = true
	}
	flags = make(map[string]string)
	for i := 0; i < len(args); {
		if strings.HasPrefix(args[i], "--") {
			name := strings.TrimPrefix(args[i], "--")
			if name == "" {
				positionals = append(positionals, args[i])
				i++
				continue
			}
			if known[name] {
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
					flags[name] = args[i+1]
					i += 2
				} else {
					flags[name] = "true"
					i++
				}
			} else {
				positionals = append(positionals, args[i])
				i++
			}
		} else {
			positionals = append(positionals, args[i])
			i++
		}
	}
	return
}

func printTable(rules []netsh.Rule, notes map[string]string) {
	const colGap = 2
	widths := [3]int{len("LISTEN"), len("CONNECT"), len("NOTE")}
	type row struct{ listen, connect, note string }
	rows := make([]row, len(rules))
	for i, r := range rules {
		rows[i] = row{
			listen:  r.ListenAddr + ":" + r.ListenPort,
			connect: r.ConnectAddr + ":" + r.ConnectPort,
			note:    notes[r.Key()],
		}
		if l := len(rows[i].listen); l > widths[0] {
			widths[0] = l
		}
		if l := len(rows[i].connect); l > widths[1] {
			widths[1] = l
		}
		if l := len(rows[i].note); l > widths[2] {
			widths[2] = l
		}
	}
	fmt.Printf("%-*s%s%-*s%s%-*s\n", widths[0], "LISTEN", strings.Repeat(" ", colGap), widths[1], "CONNECT", strings.Repeat(" ", colGap), widths[2], "NOTE")
	for _, r := range rows {
		fmt.Printf("%-*s%s%-*s%s%-*s\n", widths[0], r.listen, strings.Repeat(" ", colGap), widths[1], r.connect, strings.Repeat(" ", colGap), widths[2], r.note)
	}
}

