// Package store persists per-rule notes and backup files under %APPDATA%\ppm.
package store

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wmy2981/ppm/internal/netsh"
)

// Store manages the %APPDATA%\ppm data directory.
type Store struct {
	dir  string
	file string
}

// Dir returns the data directory path.
func (s *Store) Dir() string { return s.dir }

type notesFile struct {
	Notes map[string]string `json:"notes"` // key: "listenaddr:listenport" -> note
}

// Open creates %APPDATA%\ppm if missing and returns a handle to it.
func Open() (*Store, error) {
	appdata, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(appdata, "ppm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir, file: filepath.Join(dir, "notes.json")}, nil
}

func (s *Store) LoadNotes() (map[string]string, error) {
	data, err := os.ReadFile(s.file)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var nf notesFile
	if err := json.Unmarshal(data, &nf); err != nil {
		return nil, fmt.Errorf("corrupt %s: %w", s.file, err)
	}
	if nf.Notes == nil {
		nf.Notes = map[string]string{}
	}
	return nf.Notes, nil
}

func (s *Store) SaveNotes(notes map[string]string) error {
	data, err := json.MarshalIndent(notesFile{Notes: notes}, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.file + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.file)
}

// Export writes all current rules plus their notes to a dated JSON file and
// returns its path.
func (s *Store) Export(rules []netsh.Rule, notes map[string]string, appVersion string) (string, error) {
	type backupRule struct {
		netsh.Rule
		Note string `json:"note,omitempty"`
	}
	doc := struct {
		Version    string       `json:"version"`
		ExportedAt string       `json:"exported_at"`
		Rules      []backupRule `json:"rules"`
	}{
		Version:    appVersion,
		ExportedAt: time.Now().Format(time.RFC3339),
	}
	for _, r := range rules {
		doc.Rules = append(doc.Rules, backupRule{Rule: r, Note: notes[r.Key()]})
	}
	sort.Slice(doc.Rules, func(i, j int) bool {
		a, b := doc.Rules[i].Rule, doc.Rules[j].Rule
		if a.ListenPort == b.ListenPort {
			return strings.Compare(a.ListenAddr, b.ListenAddr) < 0
		}
		return portLess(a.ListenPort, b.ListenPort)
	})
	path := filepath.Join(s.dir, "backup-"+time.Now().Format("20060102-150405")+".json")
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, data, 0o644)
}

// ImportResult reports what a merge-import did.
type ImportResult struct {
	Created int
	Skipped int
	Failed  int
}

// Import reads a backup file created by Export and merges it into the live
// system: existing listen keys are skipped, missing ones are created. Notes
// from the backup are merged for created rules only.
func (s *Store) Import(path string, live []netsh.Rule, notes map[string]string) (*ImportResult, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var doc struct {
		Rules []struct {
			netsh.Rule
			Note string `json:"note"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, nil, fmt.Errorf("invalid backup file: %w", err)
	}
	existing := make(map[string]bool, len(live))
	for _, r := range live {
		existing[r.Key()] = true
	}
	res := &ImportResult{}
	var warnings []string
	for _, br := range doc.Rules {
		r := netsh.Rule{
			ListenAddr:  strings.TrimSpace(br.ListenAddr),
			ListenPort:  strings.TrimSpace(br.ListenPort),
			ConnectAddr: strings.TrimSpace(br.ConnectAddr),
			ConnectPort: strings.TrimSpace(br.ConnectPort),
		}
		key := r.Key()
		if net.ParseIP(r.ListenAddr) == nil || net.ParseIP(r.ConnectAddr) == nil ||
			!netsh.IsPort(r.ListenPort) || !netsh.IsPort(r.ConnectPort) {
			res.Failed++
			warnings = append(warnings, fmt.Sprintf("invalid entry skipped: %s", key))
			continue
		}
		if existing[key] {
			res.Skipped++
			continue
		}
		if err := netsh.AddRule(r); err != nil {
			res.Failed++
			warnings = append(warnings, fmt.Sprintf("create %s failed: %v", key, err))
			continue
		}
		existing[key] = true
		if br.Note != "" {
			notes[key] = br.Note
		}
		res.Created++
	}
	return res, warnings, nil
}

// NewestBackup returns the lexicographically newest backup-*.json in the
// store directory, or "" when none exists.
func (s *Store) NewestBackup() string {
	newest := ""
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "backup-") && strings.HasSuffix(name, ".json") && name > newest {
			newest = name
		}
	}
	if newest == "" {
		return ""
	}
	return filepath.Join(s.dir, newest)
}

func portLess(a, b string) bool {
	pa, _ := strconv.Atoi(a)
	pb, _ := strconv.Atoi(b)
	return pa < pb
}
