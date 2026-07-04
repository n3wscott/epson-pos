/*
Copyright 2022 Scott Nichols
SPDX-License-Identifier: Apache-2.0
*/

package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/n3wscott/escpos/pkg/transport"
)

const printerPort = "9100"

type printerManager struct {
	mu                   sync.Mutex
	configuredTarget     string
	printerMAC           string
	activeTarget         string
	lastSuccessfulTarget string
	lastScanTime         time.Time
	lastPrintError       string
	stateFile            string

	dial       func(context.Context, string) (io.WriteCloser, error)
	check      func(context.Context, string, string) bool
	scan       func(context.Context, []string, string) ([]string, error)
	now        func() time.Time
	persistLog bool
}

type printerState struct {
	ActiveTarget         string    `json:"active_target,omitempty"`
	LastSuccessfulTarget string    `json:"last_successful_target,omitempty"`
	LastScanTime         time.Time `json:"last_scan_time,omitempty"`
	LastPrintError       string    `json:"last_print_error,omitempty"`
}

type printerStatus struct {
	ConfiguredTarget     string `json:"configured_target"`
	PrinterMAC           string `json:"printer_mac,omitempty"`
	ActiveTarget         string `json:"active_target"`
	LastSuccessfulTarget string `json:"last_successful_target"`
	LastScanTime         string `json:"last_scan_time,omitempty"`
	LastPrintError       string `json:"last_print_error,omitempty"`
	Reachable            bool   `json:"reachable"`
}

type dialError struct {
	target string
	err    error
}

func (e dialError) Error() string {
	return e.err.Error()
}

func (e dialError) Unwrap() error {
	return e.err
}

func newPrinterManager(configuredTarget, printerMAC, stateFile string) (*printerManager, error) {
	configuredTarget = strings.TrimSpace(configuredTarget)
	if configuredTarget == "" {
		configuredTarget = defaultPrinterTarget
	}
	normalizedMAC, err := normalizeMAC(printerMAC)
	if err != nil {
		return nil, err
	}
	if stateFile == "" {
		stateFile = "printer_state.json"
	}
	m := &printerManager{
		configuredTarget: configuredTarget,
		printerMAC:       normalizedMAC,
		activeTarget:     configuredTarget,
		stateFile:        stateFile,
		dial:             transport.DialTCP,
		check:            checkPrinterTarget,
		scan:             scanPrinterTargets,
		now:              time.Now,
		persistLog:       true,
	}
	state, err := readPrinterState(stateFile)
	if err != nil {
		return nil, err
	}
	m.lastSuccessfulTarget = strings.TrimSpace(state.LastSuccessfulTarget)
	m.lastScanTime = state.LastScanTime
	m.lastPrintError = state.LastPrintError
	if strings.TrimSpace(state.ActiveTarget) != "" {
		m.activeTarget = strings.TrimSpace(state.ActiveTarget)
	}
	return m, nil
}

func (m *printerManager) Print(ctx context.Context, payload []byte, overrideTarget string) error {
	knownTargets := m.candidates(overrideTarget)
	if len(knownTargets) == 0 {
		return fmt.Errorf("no printer target configured")
	}

	firstTarget := knownTargets[0]
	if err := m.printOnce(ctx, firstTarget, payload); err != nil {
		var de dialError
		if !errors.As(err, &de) {
			m.setPrintError(err)
			return err
		}
		log.Printf("printer dial failed for %s: %v; scanning local networks", de.target, de.err)
		m.setPrintError(err)
		target, scanErr := m.rediscover(ctx, knownTargets)
		if scanErr != nil {
			return fmt.Errorf("%w; rediscovery failed: %v", err, scanErr)
		}
		if retryErr := m.printOnce(ctx, target, payload); retryErr != nil {
			m.setPrintError(retryErr)
			return retryErr
		}
		m.markSuccess(target)
		return nil
	}
	m.markSuccess(firstTarget)
	return nil
}

func (m *printerManager) Status(ctx context.Context) printerStatus {
	snapshot := m.snapshot()
	target := snapshot.ActiveTarget
	if target == "" {
		target = snapshot.ConfiguredTarget
	}
	if target != "" {
		checkCtx, cancel := context.WithTimeout(ctx, 600*time.Millisecond)
		defer cancel()
		snapshot.Reachable = m.check(checkCtx, target, snapshot.PrinterMAC)
	}
	return snapshot
}

func (m *printerManager) snapshot() printerStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	status := printerStatus{
		ConfiguredTarget:     m.configuredTarget,
		PrinterMAC:           m.printerMAC,
		ActiveTarget:         m.activeTarget,
		LastSuccessfulTarget: m.lastSuccessfulTarget,
		LastPrintError:       m.lastPrintError,
	}
	if !m.lastScanTime.IsZero() {
		status.LastScanTime = m.lastScanTime.Format(time.RFC3339)
	}
	return status
}

func (m *printerManager) candidates(overrideTarget string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return uniqueTargets(
		strings.TrimSpace(overrideTarget),
		m.configuredTarget,
		m.activeTarget,
		m.lastSuccessfulTarget,
	)
}

func (m *printerManager) rediscover(ctx context.Context, knownTargets []string) (string, error) {
	for _, target := range knownTargets {
		checkCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		ok := m.check(checkCtx, target, m.printerMAC)
		cancel()
		if ok {
			m.markScan(target, nil)
			return target, nil
		}
	}

	found, err := m.scan(ctx, knownTargets, m.printerMAC)
	if err != nil {
		m.markScan("", err)
		return "", err
	}
	if len(found) == 0 {
		err := fmt.Errorf("no reachable printers found on local networks")
		m.markScan("", err)
		return "", err
	}
	target := found[0]
	m.markScan(target, nil)
	return target, nil
}

func (m *printerManager) printOnce(ctx context.Context, target string, payload []byte) error {
	if m.printerMAC != "" {
		checkCtx, cancel := context.WithTimeout(ctx, 600*time.Millisecond)
		ok := m.check(checkCtx, target, m.printerMAC)
		cancel()
		if !ok {
			return dialError{target: target, err: fmt.Errorf("target %s does not match printer MAC %s", target, m.printerMAC)}
		}
	}

	conn, err := m.dial(ctx, target)
	if err != nil {
		return dialError{target: target, err: err}
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("close printer connection: %v", err)
		}
	}()

	written, err := conn.Write(payload)
	if err != nil {
		return fmt.Errorf("unable to write printer bytes to %s after %d/%d bytes: %w", target, written, len(payload), err)
	}
	if written != len(payload) {
		return fmt.Errorf("short printer write to %s: wrote %d/%d bytes", target, written, len(payload))
	}
	return nil
}

func (m *printerManager) markSuccess(target string) {
	m.mu.Lock()
	m.activeTarget = target
	m.lastSuccessfulTarget = target
	m.lastPrintError = ""
	state := m.stateLocked()
	m.mu.Unlock()
	m.persist(state)
}

func (m *printerManager) markScan(target string, err error) {
	m.mu.Lock()
	m.lastScanTime = m.now().UTC()
	if target != "" {
		m.activeTarget = target
	}
	if err != nil {
		m.lastPrintError = err.Error()
	}
	state := m.stateLocked()
	m.mu.Unlock()
	m.persist(state)
}

func (m *printerManager) setPrintError(err error) {
	m.mu.Lock()
	if err != nil {
		m.lastPrintError = err.Error()
	}
	state := m.stateLocked()
	m.mu.Unlock()
	m.persist(state)
}

func (m *printerManager) stateLocked() printerState {
	return printerState{
		ActiveTarget:         m.activeTarget,
		LastSuccessfulTarget: m.lastSuccessfulTarget,
		LastScanTime:         m.lastScanTime,
		LastPrintError:       m.lastPrintError,
	}
}

func (m *printerManager) persist(state printerState) {
	if m.stateFile == "" {
		return
	}
	if err := writePrinterState(m.stateFile, state); err != nil && m.persistLog {
		log.Printf("persist printer state: %v", err)
	}
}

func readPrinterState(path string) (printerState, error) {
	var state printerState
	if path == "" {
		return state, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("read printer state %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return state, nil
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("parse printer state %s: %w", path, err)
	}
	return state, nil
}

func writePrinterState(path string, state printerState) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

func scanPrinterTargets(ctx context.Context, preferred []string, printerMAC string) ([]string, error) {
	subnets, err := localPrinterSubnets()
	if err != nil {
		return nil, err
	}

	targets := make(chan string)
	results := make(chan string, 32)
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for target := range targets {
				checkCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
				ok := checkPrinterTarget(checkCtx, target, printerMAC)
				cancel()
				if ok {
					results <- target
				}
			}
		}()
	}

	go func() {
		defer close(targets)
		for _, target := range uniqueTargets(preferred...) {
			select {
			case <-ctx.Done():
				return
			case targets <- target:
			}
		}
		for _, subnet := range subnets {
			hosts := hostsForSubnet(subnet)
			for _, host := range hosts {
				select {
				case <-ctx.Done():
					return
				case targets <- net.JoinHostPort(host, printerPort):
				}
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	found := []string(nil)
	seen := map[string]bool{}
	for target := range results {
		if seen[target] {
			continue
		}
		seen[target] = true
		found = append(found, target)
		log.Printf("printer scan found reachable target %s", target)
	}
	sort.SliceStable(found, func(i, j int) bool {
		left := targetRank(found[i], preferred)
		right := targetRank(found[j], preferred)
		if left != right {
			return left < right
		}
		return targetLess(found[i], found[j])
	})
	if ctx.Err() != nil {
		return found, ctx.Err()
	}
	return found, nil
}

func checkPrinterTarget(ctx context.Context, target, printerMAC string) bool {
	if strings.TrimSpace(target) == "" {
		return false
	}
	dialer := net.Dialer{Timeout: 300 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		return false
	}
	_ = conn.Close()
	if printerMAC != "" && !targetHasMAC(ctx, target, printerMAC) {
		return false
	}
	return true
}

func targetHasMAC(ctx context.Context, target, printerMAC string) bool {
	normalizedMAC, err := normalizeMAC(printerMAC)
	if err != nil || normalizedMAC == "" {
		return false
	}
	host, _, err := net.SplitHostPort(target)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host).To4()
	if ip == nil {
		return false
	}
	deadline, ok := ctx.Deadline()
	for {
		if mac, ok := arpMACForIPv4(ip.String()); ok {
			return mac == normalizedMAC
		}
		if !ok || time.Now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func arpMACForIPv4(ip string) (string, bool) {
	data, err := os.ReadFile("/proc/net/arp")
	if err != nil {
		return "", false
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[0] != ip {
			continue
		}
		mac, err := normalizeMAC(fields[3])
		if err != nil || mac == "00:00:00:00:00:00" {
			return "", false
		}
		return mac, true
	}
	return "", false
}

func normalizeMAC(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	mac, err := net.ParseMAC(value)
	if err != nil {
		return "", fmt.Errorf("invalid printer MAC %q: %w", value, err)
	}
	return strings.ToLower(mac.String()), nil
}

func localPrinterSubnets() ([]net.IPNet, error) {
	subnets := []net.IPNet{
		mustIPv4Net("192.168.86.0/24"),
		mustIPv4Net("10.77.0.0/24"),
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip, _, ok := parseInterfaceAddr(addr)
			if !ok || !isLocalScanIPv4(ip) {
				continue
			}
			subnets = append(subnets, ipv4Slash24(ip))
		}
	}
	return uniqueSubnets(subnets), nil
}

func parseInterfaceAddr(addr net.Addr) (net.IP, *net.IPNet, bool) {
	ipNet, ok := addr.(*net.IPNet)
	if !ok {
		return nil, nil, false
	}
	ip := ipNet.IP.To4()
	if ip == nil {
		return nil, nil, false
	}
	return ip, ipNet, true
}

func hostsForSubnet(subnet net.IPNet) []string {
	ip := subnet.IP.To4()
	if ip == nil {
		return nil
	}
	ones, bits := subnet.Mask.Size()
	if bits != 32 {
		return nil
	}
	hosts := []string(nil)
	if ones < 24 {
		subnet = ipv4Slash24(ip)
		ip = subnet.IP.To4()
		ones = 24
	}
	total := 1 << (32 - ones)
	for i := 1; i < total-1; i++ {
		host := net.IPv4(ip[0], ip[1], ip[2], byte(int(ip[3])+i))
		hosts = append(hosts, host.String())
	}
	return hosts
}

func ipv4Slash24(ip net.IP) net.IPNet {
	ip = ip.To4()
	return net.IPNet{
		IP:   net.IPv4(ip[0], ip[1], ip[2], 0),
		Mask: net.CIDRMask(24, 32),
	}
}

func mustIPv4Net(cidr string) net.IPNet {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(err)
	}
	ipNet.IP = ip.To4()
	return *ipNet
}

func isLocalScanIPv4(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil {
		return false
	}
	if ip[0] == 10 {
		return true
	}
	if ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 {
		return true
	}
	if ip[0] == 192 && ip[1] == 168 {
		return true
	}
	if ip[0] == 169 && ip[1] == 254 {
		return true
	}
	return false
}

func uniqueTargets(values ...string) []string {
	out := []string(nil)
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func uniqueSubnets(values []net.IPNet) []net.IPNet {
	out := []net.IPNet(nil)
	seen := map[string]bool{}
	for _, value := range values {
		key := value.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func targetRank(target string, preferred []string) int {
	for i, value := range preferred {
		if strings.TrimSpace(value) == target {
			return i
		}
	}
	return len(preferred) + 1
}

func targetLess(left, right string) bool {
	leftIP, leftPort, leftOK := splitIPv4Target(left)
	rightIP, rightPort, rightOK := splitIPv4Target(right)
	if leftOK && rightOK {
		for i := 0; i < net.IPv4len; i++ {
			if leftIP[i] != rightIP[i] {
				return leftIP[i] < rightIP[i]
			}
		}
		return leftPort < rightPort
	}
	return left < right
}

func splitIPv4Target(target string) (net.IP, string, bool) {
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return nil, "", false
	}
	ip := net.ParseIP(host).To4()
	if ip == nil {
		return nil, "", false
	}
	return ip, port, true
}
