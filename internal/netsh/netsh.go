package netsh

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

const dialTimeout = 3 * time.Second

// Rule is one netsh portproxy v4tov4 entry.
type Rule struct {
	ListenAddr  string `json:"listen_address"`
	ListenPort  string `json:"listen_port"`
	ConnectAddr string `json:"connect_address"`
	ConnectPort string `json:"connect_port"`
}

func (r Rule) Key() string    { return r.ListenAddr + ":" + r.ListenPort }
func (r Rule) Target() string { return net.JoinHostPort(r.ConnectAddr, r.ConnectPort) }

// decodeConsole converts netsh/netstat output from the OEM codepage (GBK on
// zh-CN Windows) to UTF-8; pass through when input is already valid UTF-8.
func decodeConsole(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	out, err := simplifiedchinese.GBK.NewDecoder().Bytes(b)
	if err != nil {
		return string(b)
	}
	return string(out)
}

func runNetsh(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "netsh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(decodeConsole(stderr.Bytes()))
		if msg == "" {
			msg = strings.TrimSpace(decodeConsole(stdout.Bytes()))
		}
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("netsh: %s", msg)
	}
	return decodeConsole(stdout.Bytes()), nil
}

// ListRules returns all v4tov4 portproxy rules in netsh's original order.
func ListRules() ([]Rule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := runNetsh(ctx, "interface", "portproxy", "show", "v4tov4")
	if err != nil {
		return nil, err
	}
	return parsePortproxy(out), nil
}

// parsePortproxy scans for data rows: four whitespace-separated fields where
// f0/f2 are IPv4 addresses and f1/f3 are ports. Locale-specific headers never
// match this shape.
func parsePortproxy(out string) []Rule {
	var rules []Rule
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(strings.TrimSpace(line))
		if len(f) != 4 {
			continue
		}
		if net.ParseIP(f[0]) == nil || net.ParseIP(f[2]) == nil {
			continue
		}
		if !IsPort(f[1]) || !IsPort(f[3]) {
			continue
		}
		rules = append(rules, Rule{f[0], f[1], f[2], f[3]})
	}
	return rules
}

// IsPort reports whether s is a valid TCP port number (1-65535).
func IsPort(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n >= 1 && n <= 65535
}

// AddRule creates a v4tov4 forwarding rule (requires elevation).
func AddRule(r Rule) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := runNetsh(ctx, "interface", "portproxy", "add", "v4tov4",
		"listenaddress="+r.ListenAddr, "listenport="+r.ListenPort,
		"connectaddress="+r.ConnectAddr, "connectport="+r.ConnectPort)
	return err
}

// DeleteRule removes a v4tov4 forwarding rule (requires elevation).
func DeleteRule(r Rule) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := runNetsh(ctx, "interface", "portproxy", "delete", "v4tov4",
		"listenaddress="+r.ListenAddr, "listenport="+r.ListenPort)
	return err
}

// ListeningAddrs returns the set of local "ip:port" addresses currently in
// TCP LISTENING state, lowercased.
func ListeningAddrs() (map[string]bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "netstat", "-ano", "-p", "tcp")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	listening := make(map[string]bool)
	for _, line := range strings.Split(decodeConsole(stdout.Bytes()), "\n") {
		f := strings.Fields(strings.TrimSpace(line))
		if len(f) >= 4 && strings.EqualFold(f[0], "tcp") && strings.EqualFold(f[3], "LISTENING") {
			listening[strings.ToLower(f[1])] = true
		}
	}
	return listening, nil
}

// TestConnectivity dials the rule's target address.
func TestConnectivity(r Rule) (time.Duration, error) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", r.Target(), dialTimeout)
	if err != nil {
		return 0, err
	}
	conn.Close()
	return time.Since(start), nil
}
