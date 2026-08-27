package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/urfave/cli/v2"

	"github.com/wmy2981/ppm/internal/netsh"
	"github.com/wmy2981/ppm/internal/store"
	"github.com/wmy2981/ppm/internal/ui"
)

func listAction(c *cli.Context) error {
	_, notes, rules, err := openStore()
	if err != nil {
		return err
	}
	if c.Bool("json") {
		type item struct {
			netsh.Rule
			Note string `json:"note,omitempty"`
		}
		var items []item
		items = make([]item, 0, len(rules))
		for _, r := range rules {
			items = append(items, item{Rule: r, Note: notes[r.Key()]})
		}
		data, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	if len(rules) == 0 {
		fmt.Println("No rules found.")
		return nil
	}
	printTable(rules, notes)
	return nil
}

func addAction(c *cli.Context) error {
	requireElevate(c)
	lAddr, lPort, err := parseListenArg(c.String("listen"))
	if err != nil {
		return err
	}
	cAddr, cPort, err := parseListenArg(c.String("connect"))
	if err != nil {
		return err
	}
	r := netsh.Rule{
		ListenAddr:  lAddr,
		ListenPort:  lPort,
		ConnectAddr: cAddr,
		ConnectPort: cPort,
	}
	if err := netsh.AddRule(r); err != nil {
		return err
	}
	if note := c.String("note"); note != "" {
		st, err := store.Open()
		if err == nil {
			notes, _ := st.LoadNotes()
			if notes == nil {
				notes = map[string]string{}
			}
			notes[r.Key()] = note
			_ = st.SaveNotes(notes)
		}
	}
	fmt.Printf("Added rule: %s -> %s\n", r.Key(), r.Target())
	return nil
}

func editAction(c *cli.Context) error {
	requireElevate(c)
	if c.NArg() < 1 {
		return fmt.Errorf("usage: ppm edit <listen> --connect <addr:port> [--note <text>]")
	}
	addr, port, err := parseListenArg(c.Args().First())
	if err != nil {
		return err
	}
	st, notes, rules, err := openStore()
	if err != nil {
		return err
	}
	old := findRule(rules, addr, port)
	if old == nil {
		return fmt.Errorf("no rule found for %s", addr+":"+port)
	}
	newConnect := c.String("connect")
	if newConnect == "" {
		newConnect = old.Target()
	}
	cAddr, cPort, err := parseListenArg(newConnect)
	if err != nil {
		return err
	}
	newListen := c.String("listen")
	var newAddr, newPort string
	if newListen != "" {
		newAddr, newPort, err = parseListenArg(newListen)
		if err != nil {
			return err
		}
	} else {
		newAddr, newPort = old.ListenAddr, old.ListenPort
	}
	newNote := c.String("note")
	if newNote == "" {
		newNote = notes[old.Key()]
	}
	if err := netsh.DeleteRule(*old); err != nil {
		return fmt.Errorf("delete old rule: %w", err)
	}
	created := netsh.Rule{
		ListenAddr:  newAddr,
		ListenPort:  newPort,
		ConnectAddr: cAddr,
		ConnectPort: cPort,
	}
	if err := netsh.AddRule(created); err != nil {
		_ = netsh.AddRule(*old)
		return fmt.Errorf("create new rule: %w (old rule restored)", err)
	}
	delete(notes, old.Key())
	notes[created.Key()] = newNote
	_ = st.SaveNotes(notes)
	fmt.Printf("Edited rule: %s -> %s\n", old.Key(), created.Key())
	return nil
}

func deleteAction(c *cli.Context) error {
	requireElevate(c)
	if c.NArg() < 1 {
		return fmt.Errorf("usage: ppm delete <listen>")
	}
	addr, port, err := parseListenArg(c.Args().First())
	if err != nil {
		return err
	}
	st, notes, rules, err := openStore()
	if err != nil {
		return err
	}
	r := findRule(rules, addr, port)
	if r == nil {
		return fmt.Errorf("no rule found for %s", addr+":"+port)
	}
	if err := netsh.DeleteRule(*r); err != nil {
		return err
	}
	delete(notes, r.Key())
	_ = st.SaveNotes(notes)
	fmt.Printf("Deleted rule: %s\n", r.Key())
	return nil
}

func testAction(c *cli.Context) error {
	_, _, rules, err := openStore()
	if err != nil {
		return err
	}
	var targets []netsh.Rule
	if c.Bool("all") {
		targets = rules
	} else if c.NArg() > 0 {
		addr, port, err := parseListenArg(c.Args().First())
		if err != nil {
			return err
		}
		r := findRule(rules, addr, port)
		if r == nil {
			return fmt.Errorf("no rule found for %s", addr+":"+port)
		}
		targets = []netsh.Rule{*r}
	} else {
		return fmt.Errorf("usage: ppm test <listen> or ppm test --all")
	}
	type testResult struct {
		Listen string `json:"listen"`
		Target string `json:"target"`
		Latency string `json:"latency,omitempty"`
		Error  string `json:"error,omitempty"`
	}
	results := make([]testResult, 0, len(targets))
	for _, r := range targets {
		d, err := netsh.TestConnectivity(r)
		res := testResult{
			Listen: r.Key(),
			Target: r.Target(),
		}
		if err != nil {
			res.Error = err.Error()
		} else {
			res.Latency = fmt.Sprintf("%dms", d.Milliseconds())
		}
		results = append(results, res)
		if !c.Bool("json") {
			if res.Error != "" {
				fmt.Printf("%s -> %s: FAIL (%s)\n", res.Listen, res.Target, res.Error)
			} else {
				fmt.Printf("%s -> %s: OK (%s)\n", res.Listen, res.Target, res.Latency)
			}
		}
	}
	if c.Bool("json") {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	}
	return nil
}

func exportAction(c *cli.Context) error {
	st, notes, rules, err := openStore()
	if err != nil {
		return err
	}
	if output := c.String("output"); output != "" {
		type backupRule struct {
			netsh.Rule
			Note string `json:"note,omitempty"`
		}
		doc := struct {
			Version    int          `json:"version"`
			ExportedAt string       `json:"exported_at"`
			Rules      []backupRule `json:"rules"`
		}{
			Version:    1,
			ExportedAt: time.Now().Format(time.RFC3339),
		}
		for _, r := range rules {
			doc.Rules = append(doc.Rules, backupRule{Rule: r, Note: notes[r.Key()]})
		}
		data, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(output, data, 0o644); err != nil {
			return err
		}
		fmt.Printf("Exported %d rules to %s\n", len(rules), output)
		return nil
	}
	path, err := st.Export(rules, notes)
	if err != nil {
		return err
	}
	fmt.Printf("Exported %d rules to %s\n", len(rules), path)
	return nil
}

func importAction(c *cli.Context) error {
	requireElevate(c)
	if c.NArg() < 1 {
		return fmt.Errorf("usage: ppm import <filepath>")
	}
	st, notes, live, err := openStore()
	if err != nil {
		return err
	}
	res, warnings, err := st.Import(c.Args().First(), live, notes)
	if err != nil {
		return err
	}
	_ = st.SaveNotes(notes)
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	fmt.Printf("Imported: %d created, %d skipped, %d failed\n", res.Created, res.Skipped, res.Failed)
	return nil
}

func tuiAction(c *cli.Context) error {
	st, err := store.Open()
	if err != nil {
		return fmt.Errorf("open data dir: %w", err)
	}
	notes, err := st.LoadNotes()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		notes = map[string]string{}
	}
	p := tea.NewProgram(ui.New(version, st, notes), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}
