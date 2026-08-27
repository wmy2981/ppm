package cli

import (
	"encoding/json"
	"errors"
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
	// Extract positionals and leftover flags from c.Args() (handles flags after positional args).
	positionals, extra := leftoverFlags(c.Args().Slice(), "listen", "connect", "note")

	// Resolve listen: positional[0] > --listen flag > extra --listen
	var listenArg string
	if len(positionals) > 0 {
		listenArg = positionals[0]
	} else if v := c.String("listen"); v != "" {
		listenArg = v
	} else if v := extra["listen"]; v != "" {
		listenArg = v
	}
	if listenArg == "" {
		return fmt.Errorf("usage: ppm add <listen> <connect> [note]\n       ppm add --listen <addr:port> --connect <addr:port> [--note <text>]")
	}
	lAddr, lPort, err := parseListenArg(listenArg)
	if err != nil {
		return err
	}

	// Resolve connect: positional[1] > --connect flag > extra --connect
	var connectArg string
	if len(positionals) > 1 {
		connectArg = positionals[1]
	} else if v := c.String("connect"); v != "" {
		connectArg = v
	} else if v := extra["connect"]; v != "" {
		connectArg = v
	}
	if connectArg == "" {
		return errors.New(failf("connect address is required"))
	}
	cAddr, cPort, err := parseListenArg(connectArg)
	if err != nil {
		return err
	}

	// Resolve note: positional[2] > --note flag > extra --note
	var note string
	if len(positionals) > 2 {
		note = positionals[2]
	} else if v := c.String("note"); v != "" {
		note = v
	} else if v := extra["note"]; v != "" {
		note = v
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
	if note != "" {
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
	fmt.Println(successf("Added rule: %s -> %s", r.Key(), r.Target()))
	return nil
}

func editAction(c *cli.Context) error {
	// Resolve origin listen: first raw arg (required)
	if c.NArg() < 1 {
		return fmt.Errorf("usage: ppm edit <originlisten> [<listen>] [<connect>] [note]\n       ppm edit <listen> --connect <addr:port> [--note <text>]")
	}
	addr, port, err := parseListenArg(c.Args().Get(0))
	if err != nil {
		return err
	}

	st, notes, rules, err := openStore()
	if err != nil {
		return err
	}
	old := findRule(rules, addr, port)
	if old == nil {
		return errors.New(failf("no rule found for %s", addr+":"+port))
	}

	// Extract positionals and leftover flags from ALL args (handles flags after positional args).
	positionals, extra := leftoverFlags(c.Args().Slice(), "listen", "connect", "note")
	// positionals[0] is originlisten (already parsed above); remaining are new values.
	positionals = positionals[1:]

	// Resolve new listen: positional[0] > --listen flag > extra --listen > original
	var newListen string
	if len(positionals) > 0 {
		newListen = positionals[0]
	} else if v := c.String("listen"); v != "" {
		newListen = v
	} else if v := extra["listen"]; v != "" {
		newListen = v
	}
	var newAddr, newPort string
	if newListen != "" {
		newAddr, newPort, err = parseListenArg(newListen)
		if err != nil {
			return err
		}
	} else {
		newAddr, newPort = old.ListenAddr, old.ListenPort
	}

	// Resolve new connect: positional[1] > --connect flag > extra --connect > original
	var newConnect string
	if len(positionals) > 1 {
		newConnect = positionals[1]
	} else if v := c.String("connect"); v != "" {
		newConnect = v
	} else if v := extra["connect"]; v != "" {
		newConnect = v
	}
	if newConnect == "" {
		newConnect = old.Target()
	}
	cAddr, cPort, err := parseListenArg(newConnect)
	if err != nil {
		return err
	}

	// Resolve new note: positional[2] > --note flag > extra --note > original
	var newNote string
	if len(positionals) > 2 {
		newNote = positionals[2]
	} else if v := c.String("note"); v != "" {
		newNote = v
	} else if v := extra["note"]; v != "" {
		newNote = v
	}
	if newNote == "" {
		newNote = notes[old.Key()]
	}

	if err := netsh.DeleteRule(*old); err != nil {
		return errors.New(failf("delete old rule: %v", err))
	}
	created := netsh.Rule{
		ListenAddr:  newAddr,
		ListenPort:  newPort,
		ConnectAddr: cAddr,
		ConnectPort: cPort,
	}
	if err := netsh.AddRule(created); err != nil {
		_ = netsh.AddRule(*old)
		return errors.New(failf("create new rule: %v (old rule restored)", err))
	}
	delete(notes, old.Key())
	notes[created.Key()] = newNote
	_ = st.SaveNotes(notes)
	fmt.Println(successf("Edited rule: %s -> %s", old.Key(), created.Key()))
	return nil
}

func deleteAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("usage: ppm delete <listen> [<listen> ...]")
	}
	st, notes, rules, err := openStore()
	if err != nil {
		return err
	}

	var errs []string
	deleted := 0
	for i := 0; i < c.NArg(); i++ {
		addr, port, err := parseListenArg(c.Args().Get(i))
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		r := findRule(rules, addr, port)
		if r == nil {
			errs = append(errs, fmt.Sprintf("no rule found for %s", addr+":"+port))
			continue
		}
		if err := netsh.DeleteRule(*r); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		delete(notes, r.Key())
		deleted++
		fmt.Println(successf("Deleted rule: %s", r.Key()))
	}

	if deleted > 0 {
		_ = st.SaveNotes(notes)
	}
	if len(errs) > 0 {
		for _, msg := range errs {
			fmt.Fprintln(os.Stderr, failf("%s", msg))
		}
		return errors.New(failf("%d rule(s) failed to delete", len(errs)))
	}
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
			return errors.New(failf("no rule found for %s", addr+":"+port))
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
				fmt.Printf("%s -> %s: %s\n", res.Listen, res.Target, failf("FAIL (%s)", res.Error))
			} else {
				fmt.Printf("%s -> %s: %s\n", res.Listen, res.Target, styleOK.Render("OK ("+res.Latency+")"))
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
			Version    string       `json:"version"`
			ExportedAt string       `json:"exported_at"`
			Rules      []backupRule `json:"rules"`
		}{
			Version:    version,
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
		fmt.Println(successf("Exported %d rules to %s", len(rules), output))
		return nil
	}
	path, err := st.Export(rules, notes, version)
	if err != nil {
		return err
	}
	fmt.Println(successf("Exported %d rules to %s", len(rules), path))
	return nil
}

func importAction(c *cli.Context) error {
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
		fmt.Fprintln(os.Stderr, warnf("%s", w))
	}
	fmt.Println(successf("Imported: %d created, %d skipped, %d failed", res.Created, res.Skipped, res.Failed))
	return nil
}

func tuiAction(c *cli.Context) error {
	st, err := store.Open()
	if err != nil {
		return errors.New(failf("open data dir: %v", err))
	}
	notes, err := st.LoadNotes()
	if err != nil {
		fmt.Fprintln(os.Stderr, warnf("load notes: %v", err))
		notes = map[string]string{}
	}
	p := tea.NewProgram(ui.New(version, st, notes), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return errors.New(failf("tui: %v", err))
	}
	return nil
}
