package cli

import (
	"reflect"
	"testing"

	"github.com/wmy2981/ppm/internal/netsh"
)

func TestParseListenArg(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		addr    string
		port    string
		wantErr bool
	}{
		{"short form :port", ":8080", "0.0.0.0", "8080", false},
		{"explicit 0.0.0.0", "0.0.0.0:8080", "0.0.0.0", "8080", false},
		{"specific IP", "192.168.1.1:80", "192.168.1.1", "80", false},
		{"private IP", "10.0.0.1:443", "10.0.0.1", "443", false},
		{"port 1 (lower bound)", ":1", "0.0.0.0", "1", false},
		{"port 65535 (upper bound)", ":65535", "0.0.0.0", "65535", false},
		{"no colon", "8080", "", "", true},
		{"port 0", ":0", "", "", true},
		{"port too high", ":99999", "", "", true},
		{"non-numeric port", ":abc", "", "", true},
		{"empty port", "192.168.1.1:", "", "", true},
		{"just colon", ":", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, port, err := parseListenArg(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseListenArg(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr {
				if addr != tt.addr {
					t.Errorf("addr = %q, want %q", addr, tt.addr)
				}
				if port != tt.port {
					t.Errorf("port = %q, want %q", port, tt.port)
				}
			}
		})
	}
}

func TestLeftoverFlags(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		flagNames []string
		wantPos   []string
		wantFlags map[string]string
	}{
		{
			name:      "long flag with value",
			args:      []string{"--listen", ":8080"},
			flagNames: []string{"listen"},
			wantPos:   nil,
			wantFlags: map[string]string{"listen": ":8080"},
		},
		{
			name:      "short flag with value",
			args:      []string{"-l", ":8080"},
			flagNames: []string{"listen"},
			wantPos:   nil,
			wantFlags: map[string]string{"listen": ":8080"},
		},
		{
			name:      "positionals before flags",
			args:      []string{":8080", "10.0.0.1:80", "--note", "web"},
			flagNames: []string{"listen", "connect", "note"},
			wantPos:   []string{":8080", "10.0.0.1:80"},
			wantFlags: map[string]string{"note": "web"},
		},
		{
			name:      "mixed long and short flags",
			args:      []string{"-l", ":8080", "--connect", "10.0.0.1:80", "-n", "web"},
			flagNames: []string{"listen", "connect", "note"},
			wantPos:   nil,
			wantFlags: map[string]string{"listen": ":8080", "connect": "10.0.0.1:80", "note": "web"},
		},
		{
			name:      "boolean flag (no value)",
			args:      []string{"--json"},
			flagNames: []string{"json"},
			wantPos:   nil,
			wantFlags: map[string]string{"json": "true"},
		},
		{
			name:      "unknown flag treated as positional",
			args:      []string{"--unknown", "val", ":8080"},
			flagNames: []string{"listen"},
			wantPos:   []string{"--unknown", "val", ":8080"},
			wantFlags: map[string]string{},
		},
		{
			name:      "no flags",
			args:      []string{":8080", "10.0.0.1:80"},
			flagNames: []string{"listen", "connect"},
			wantPos:   []string{":8080", "10.0.0.1:80"},
			wantFlags: map[string]string{},
		},
		{
			name:      "empty input",
			args:      []string{},
			flagNames: []string{"listen"},
			wantPos:   nil,
			wantFlags: map[string]string{},
		},
		{
			name:      "flag without value at end",
			args:      []string{"--listen"},
			flagNames: []string{"listen"},
			wantPos:   nil,
			wantFlags: map[string]string{"listen": "true"},
		},
		{
			name:      "flag whose next arg looks like a flag",
			args:      []string{"--listen", "--connect"},
			flagNames: []string{"listen", "connect"},
			wantPos:   nil,
			wantFlags: map[string]string{"listen": "true", "connect": "true"},
		},
		{
			name:      "short flag with dash value",
			args:      []string{"-l", ":8080", "-c", "10.0.0.1:80"},
			flagNames: []string{"listen", "connect"},
			wantPos:   nil,
			wantFlags: map[string]string{"listen": ":8080", "connect": "10.0.0.1:80"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPos, gotFlags := leftoverFlags(tt.args, tt.flagNames...)
			if !reflect.DeepEqual(gotPos, tt.wantPos) {
				t.Errorf("positionals = %v, want %v", gotPos, tt.wantPos)
			}
			if !reflect.DeepEqual(gotFlags, tt.wantFlags) {
				t.Errorf("flags = %v, want %v", gotFlags, tt.wantFlags)
			}
		})
	}
}

func TestFindRule(t *testing.T) {
	rules := []netsh.Rule{
		{ListenAddr: "0.0.0.0", ListenPort: "8080", ConnectAddr: "10.0.0.1", ConnectPort: "80"},
		{ListenAddr: "192.168.1.1", ListenPort: "443", ConnectAddr: "10.0.0.2", ConnectPort: "443"},
		{ListenAddr: "10.0.0.1", ListenPort: "80", ConnectAddr: "10.0.0.3", ConnectPort: "80"},
	}

	tests := []struct {
		addr, port string
		found      bool
	}{
		{"0.0.0.0", "8080", true},
		{"192.168.1.1", "443", true},
		{"10.0.0.1", "80", true},
		{"0.0.0.0", "9090", false},
		{"192.168.1.2", "443", false},
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.addr+":"+tt.port, func(t *testing.T) {
			r := findRule(rules, tt.addr, tt.port)
			if (r != nil) != tt.found {
				t.Errorf("findRule(%q, %q) found = %v, want %v", tt.addr, tt.port, r != nil, tt.found)
			}
		})
	}
}
