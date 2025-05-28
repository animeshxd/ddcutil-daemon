package main

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	socketPath      = "/tmp/brightness.sock"
	brightnessVCP   = "10"
	powerVCP        = "d6"
	monitorID       = "1"
	debounceTime    = 300 * time.Millisecond
	maxBrightness   = 100
	powerOn         = "01"
	powerOff        = "04"
	brightnessStep  = 10
	waybarSignalNum = "5"
)

var (
	mu       sync.Mutex
	incCount int
	decCount int
	logger   *slog.Logger
)

func getCurrentBrightness() (current int, max int, err error) {
	out, err := exec.Command("ddcutil", "getvcp", brightnessVCP, "--brief", "--display", monitorID).Output()
	if err != nil {
		return 0, 0, err
	}
	_, err = fmt.Sscanf(string(out), "VCP 10 C %d %d", &current, &max)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse brightness output: %v", err)
	}
	return current, max, nil
}

func adjustBrightnessValue(value int) {
	logger.Info("adjusting brightness", "value", value)
	cmd := exec.Command("ddcutil", "setvcp", brightnessVCP, fmt.Sprint(value), "--display", monitorID)
	err := cmd.Run()
	if err != nil {
		logger.Error("failed to set brightness", "error", err)
		return
	}
	signalWaybarUpdate()
}

func signalWaybarUpdate() error {
	logger.Debug("sending waybar reload signal")
	err := exec.Command("pkill", "-SIGRTMIN+"+waybarSignalNum, "waybar").Run()
	if err != nil {
		logger.Error("failed to reload waybar module", "error", err)
		return err
	}
	return nil
}

func setMonitorPower(powerState string) error {
	logger.Info("setting monitor power state", "state", powerState)
	cmd := exec.Command("ddcutil", "setvcp", powerVCP, powerState, "--display", monitorID)
	err := cmd.Run()
	if err != nil {
		logger.Error("failed to set monitor power state", "error", err)
		return err
	}
	return nil
}

func putMonitorToSleep() error {
	return setMonitorPower(powerOff)
}

func wakeupMonitor() error {
	return setMonitorPower(powerOn)
}

func processCoalescedCommands() {
	for {
		time.Sleep(debounceTime)
		mu.Lock()
		i, d := incCount, decCount
		incCount, decCount = 0, 0
		mu.Unlock()

		if i == 0 && d == 0 {
			continue
		}

		logger.Info("processing coalesced brightness commands", "increase", i, "decrease", d)

		curr, max, err := getCurrentBrightness()
		if err != nil {
			logger.Error("failed to get current brightness", "error", err)
			continue
		}

		newVal := curr + (i * brightnessStep) - (d * brightnessStep)
		if newVal > max {
			newVal = max
		} else if newVal < 0 {
			newVal = 0
		}
		go adjustBrightnessValue(newVal)
	}
}

func handleClientConnection(conn net.Conn) {
	defer conn.Close()
	logger.Info("client connected")
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		cmd := strings.TrimSpace(scanner.Text())
		logger.Debug("received command", "command", cmd)

		switch cmd {
		case "inc":
			mu.Lock()
			incCount++
			mu.Unlock()
			conn.Write([]byte("ok\n"))
			return
		case "dec":
			mu.Lock()
			decCount++
			mu.Unlock()
			conn.Write([]byte("ok\n"))
			return
		case "get":
			current, max, err := getCurrentBrightness()
			if err != nil {
				logger.Error("failed to get brightness for get command", "error", err)
				conn.Write([]byte("error\n"))
				return
			}
			if max == 0 {
				max = maxBrightness
			}
			percent := (current * 100) / max
			if percent == 0 {
				percent = 1
			}
			response := fmt.Sprintf("{\"percentage\": %d}\n", percent)
			conn.Write([]byte(response))
			return
		case "sleep":
			err := putMonitorToSleep()
			if err != nil {
				conn.Write([]byte("error\n"))
			} else {
				conn.Write([]byte("ok\n"))
			}
			return
		case "wakeup":
			err := wakeupMonitor()
			if err != nil {
				conn.Write([]byte("error\n"))
			} else {
				conn.Write([]byte("ok\n"))
			}
			return
		default:
			logger.Warn("unknown command received", "command", cmd)
			conn.Write([]byte("Invalid command\n"))
			return
		}
	}
}

func setupLogger() {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	handler := slog.NewTextHandler(os.Stdout, opts)
	logger = slog.New(handler)
}

func cleanupExistingSocket() error {
	if err := os.RemoveAll(socketPath); err != nil {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}
	return nil
}

func createUnixSocket() (net.Listener, error) {
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("socket creation error: %w", err)
	}

	if err := os.Chmod(socketPath, 0666); err != nil {
		listener.Close()
		return nil, fmt.Errorf("failed to set socket permissions: %w", err)
	}

	return listener, nil
}

func main() {
	setupLogger()

	if err := cleanupExistingSocket(); err != nil {
		logger.Error("socket cleanup failed", "error", err)
		os.Exit(1)
	}

	listener, err := createUnixSocket()
	if err != nil {
		logger.Error("socket setup failed", "error", err)
		os.Exit(1)
	}
	defer listener.Close()

	go processCoalescedCommands()

	logger.Info("ddcutil daemon started", "socket", socketPath)
	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Error("connection accept error", "error", err)
			continue
		}
		go handleClientConnection(conn)
	}
}
