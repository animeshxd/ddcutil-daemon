package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	socketPath    = "/tmp/brightness.sock"
	vcCode        = "10"
	monitorID     = "1"
	debounceTime  = 300 * time.Millisecond
	maxBrightness = 100
)

var (
	mu       sync.Mutex
	incCount int
	decCount int
)

func getBrightness() (current int, max int, err error) {
	out, err := exec.Command("ddcutil", "getvcp", vcCode, "--brief", "--display", monitorID).Output()
	if err != nil {
		return 0, 0, err
	}
	_, err = fmt.Sscanf(string(out), "VCP 10 C %d %d", &current, &max)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse brightness output: %v", err)
	}
	return current, max, nil
}

func setBrightness(value int) {
	log.Printf("Setting brightness to %d%%", value)
	cmd := exec.Command("ddcutil", "setvcp", vcCode, fmt.Sprint(value), "--display", monitorID)
	err := cmd.Run()
	if err != nil {
		log.Printf("Failed to set brightness: %v", err)
	}
	notifyWaybar()
}

func notifyWaybar() error {
	log.Println("Reloading waybar with -SIGRTMIN+5")
	err := exec.Command("pkill", "-SIGRTMIN+5", "waybar").Run()
	if err != nil {
		fmt.Errorf("failed to reload module %v", err)
	}
	return err
}

func debounceLoop() {
	for {
		time.Sleep(debounceTime)
		mu.Lock()
		i, d := incCount, decCount
		incCount, decCount = 0, 0
		mu.Unlock()

		if i == 0 && d == 0 {
			continue
		}

		log.Printf("Coalesced commands: +%d, -%d", i, d)

		curr, max, _ := getBrightness()

		newVal := curr + (i * 10) - (d * 10)
		if newVal > max {
			newVal = max
		} else if newVal < 0 {
			newVal = 0
		}
		go setBrightness(newVal)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	log.Printf("Client connected")
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		cmd := strings.TrimSpace(scanner.Text())
		log.Printf("Received: %q", cmd)

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
			current, max, _ := getBrightness()
			if max == 0 {
				max = maxBrightness
			}
			percent := (current * 100) / max
			if percent == 0 {
				percent = 1
			}
			conn.Write(fmt.Appendf(nil, "{\"percentage\": %d}\n", percent))
			return
		default:
			log.Printf("Unknown command: %q", cmd)
			conn.Write([]byte("Invalid command\n"))
			return
		}
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if err := os.RemoveAll(socketPath); err != nil {
		log.Fatalf("Failed to remove existing socket: %v", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("Socket error: %v", err)
	}
	defer listener.Close()
	os.Chmod(socketPath, 0666)

	go debounceLoop()

	log.Println("Mock brightness server running at", socketPath)
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
		}
		go handleConnection(conn)
	}
}
