package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"time"
)

func main() {
	fmt.Println("🚀 Starting MCPWatch Integration Testing Agent...")
	fmt.Println("This agent runs mcpwatch in stdio wrapper mode using test/server.go, and simulates live traffic.")

	// 1. Compile the mock server and mcpwatch first to ensure clean execution
	fmt.Println("🔨 Compiling components...")
	buildMock := exec.Command("go", "build", "-o", "test/testserver.exe", "./test/server.go")
	if err := buildMock.Run(); err != nil {
		log.Fatalf("failed to compile mock server: %v", err)
	}

	buildWatch := exec.Command("go", "build", "-o", "mcpwatch.exe", ".")
	if err := buildWatch.Run(); err != nil {
		log.Fatalf("failed to compile mcpwatch: %v", err)
	}

	// 2. Start mcpwatch wrapping the testserver
	// Run on dashboard UI port 8080 and aggressive GOGC 35% target
	cmd := exec.Command("./mcpwatch.exe", "--wrap", "./test/testserver.exe", "--ui", "8080", "--gc-percent", "35")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatalf("stdin pipe: %v", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("stdout pipe: %v", err)
	}

	// Redirect stderr of mcpwatch to host stderr so we see runtime logs
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Fatalf("failed to start mcpwatch process: %v", err)
	}
	defer cmd.Process.Kill()

	// 3. Monitor stdout responses concurrently
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			fmt.Printf("📥 [Client Received stdout]: %s\n", scanner.Text())
		}
	}()

	time.Sleep(2 * time.Second) // wait for server to initialize
	fmt.Println("\n📡 Dashboard is live at http://localhost:8080")
	fmt.Println("Sending automated JSON-RPC traffic. Press Ctrl-C to terminate...\n")

	// 4. Generate automated mock MCP JSON-RPC traffic
	requests := []string{
		// Method tools/list
		`{"jsonrpc":"2.0","id":101,"method":"tools/list"}`,
		// Tool call (success)
		`{"jsonrpc":"2.0","id":102,"method":"tools/call","params":{"name":"calculate","arguments":{}}}`,
		// Tool call (success)
		`{"jsonrpc":"2.0","id":103,"method":"tools/call","params":{"name":"calculate","arguments":{}}}`,
		// Method query for an unknown tool/method (triggers error response)
		`{"jsonrpc":"2.0","id":104,"method":"unknown/endpoint"}`,
		// Notification (no ID, should not expect response)
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`,
	}

	// Loop to send messages and keep the traffic alive
	counter := 200
	for {
		for _, req := range requests {
			fmt.Printf("📤 [Client Sent stdin]: %s\n", req)
			_, err := io.WriteString(stdin, req+"\n")
			if err != nil {
				fmt.Printf("Failed to write to stdin: %v\n", err)
				return
			}
			time.Sleep(1500 * time.Millisecond) // interval between requests
		}

		// Inject load generation (burst of 50 fast requests) to stress-test metrics & GC
		fmt.Println("\n⚡ Injecting high-volume load burst (50 requests)...")
		for i := 0; i < 50; i++ {
			burstReq := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/list"}`, counter)
			_, _ = io.WriteString(stdin, burstReq+"\n")
			counter++
			time.Sleep(100 * time.Millisecond)
		}
		fmt.Println("⚡ Load burst completed. Resuming normal loop.\n")
		time.Sleep(2 * time.Second)
	}
}
