package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestMain runs the tests and starts the backends and the proxy server
func TestMain(m *testing.M) {
	runBackendsAndProxy()
	time.Sleep(3 * time.Second)
	code := m.Run()
	os.Exit(code)
}

func TestAuth(t *testing.T) {
	req, err := http.NewRequest("GET", "http://localhost:8080", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("Expected status 403, got %v", resp.Status)
	}
}

func TestHttpRoundRobin(t *testing.T) {

	port1 := testSingleHttpConn("http://localhost:8080", t)
	port2 := testSingleHttpConn("http://localhost:8080", t)
	port3 := testSingleHttpConn("http://localhost:8080", t)

	// Check if the messages were received on different ports
	if port1 == port2 || port2 == port3 || port1 == port3 {
		t.Errorf("Round-robin failed. Ports: %s, %s, %s", port1, port2, port3)
	}
}

func TestHttpsRoundRobin(t *testing.T) {

	port1 := testSingleHttpConn("https://localhost:8443", t)
	port2 := testSingleHttpConn("https://localhost:8443", t)
	port3 := testSingleHttpConn("https://localhost:8443", t)

	// Check if the messages were received on different ports
	if port1 == port2 || port2 == port3 || port1 == port3 {
		t.Errorf("Round-robin failed. Ports: %s, %s, %s", port1, port2, port3)
	}
}

func testSingleHttpConn(url string, t *testing.T) string {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Add("X-Auth-Token", "token1")
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Received non-200 response: %v", resp.Status)
	}

	body := make([]byte, 100)
	n, _ := resp.Body.Read(body)

	fmt.Println(string(body[:n]))
	return extractPortNumberFromHttpResponse(string(body[:n]))
}

func TestWebsocketsRoundRobin(t *testing.T) {

	u := url.URL{Scheme: "ws", Host: "localhost:8080", Path: "/websocket"}
	auth := "token1"
	headers := http.Header{"X-Auth-Token": {auth}}

	// Send a message to the server
	// Receive a message from the server
	// Check if the received message matches the sent message
	port1 := testWsSingleConn(u, headers, t)
	port2 := testWsSingleConn(u, headers, t)
	port3 := testWsSingleConn(u, headers, t)

	// Check if the messages were received on different ports
	if port1 == port2 || port2 == port3 || port1 == port3 {
		t.Errorf("Round-robin failed. Ports: %s, %s, %s", port1, port2, port3)
	}
}

func TestGracefulShutdown(t *testing.T) {
	u := url.URL{Scheme: "ws", Host: "localhost:8080", Path: "/websocket"}
	auth := "token1"
	headers := http.Header{"X-Auth-Token": {auth}}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), headers)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket server: %v", err)
	}
	defer conn.Close()
	defer conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))

	message := "Hello, server! Wait for 15 seconds"
	// this message will make the server wait for 15 seconds before responding
	err = conn.WriteMessage(websocket.TextMessage, []byte(message))
	if err != nil {
		t.Fatalf("F ailed to send message to server: %v", err)
	}

	// Send sigterm to the proxy server
	err = exec.Command("sh", "-c", `lsof -ti:8080 | xargs ps -ef | awk '$8~"main" {print $2}' | xargs kill -15`).Run()
	if err != nil {
		log.Printf("Failed to kill the process. It's OK becase of pending WS connections: %v", err)
	}

	// We should successfully receive the message from the server within the timeout period (20 seconds)
	_, receivedMessage, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to receive message from server: %v", err)
	}

	conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))

	fmt.Println(string(receivedMessage))
	if !strings.Contains(string(receivedMessage), message) {
		t.Errorf("Received message does not match the sent message. Expected: %s, Got: %s", message, string(receivedMessage))
	}
}

func testWsSingleConn(u url.URL, headers http.Header, t *testing.T) string {
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), headers)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket server: %v", err)
	}
	defer conn.Close()
	defer conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))

	message := "Hello, server!"
	err = conn.WriteMessage(websocket.TextMessage, []byte(message))
	if err != nil {
		t.Fatalf("Failed to send message to server: %v", err)
	}

	_, receivedMessage, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to receive message from server: %v", err)
	}

	fmt.Println(string(receivedMessage))
	if !strings.Contains(string(receivedMessage), message) {
		t.Errorf("Received message does not match the sent message. Expected: %s, Got: %s", message, string(receivedMessage))
	}

	return extractPortNumberFromWsResponse(string(receivedMessage))
}

func extractPortNumberFromWsResponse(message string) string {
	re := regexp.MustCompile(`Received message on port (\d+):`)
	matches := re.FindStringSubmatch(message)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func extractPortNumberFromHttpResponse(message string) string {
	re := regexp.MustCompile(`ok from port (\d+)`)
	matches := re.FindStringSubmatch(message)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// Run the backends and the proxy server
func runBackendsAndProxy() {

	cmd1 := exec.Command("sh", "-c", "./run_backends.sh")
	startCmdAndStreamOutput(cmd1)

	err := exec.Command("sh", "-c", "lsof -ti:8080 | xargs kill -9").Run()
	if err != nil {
		log.Fatalf("Failed to kill the process: %v", err)
	}
	err = exec.Command("sh", "-c", "lsof -ti:8443 | xargs kill -9").Run()
	if err != nil {
		log.Fatalf("Failed to kill the process: %v", err)
	}

	cmd2 := exec.Command("sh", "-c", "cd .. && go run main.go")
	startCmdAndStreamOutput(cmd2)
}

func startCmdAndStreamOutput(cmd *exec.Cmd) *os.Process {
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Fatalf("Failed to get  the stderr pipe: %v", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to get the stdout pipe: %v", err)
	}

	err = cmd.Start()
	if err != nil {
		log.Fatalf("Failed to run the cmd: %v", err)
	}

	scannerErr := bufio.NewScanner(stderr)
	go func() {
		for scannerErr.Scan() {
			fmt.Println(scannerErr.Text())
		}
	}()

	scannerOut := bufio.NewScanner(stdout)
	go func() {
		for scannerOut.Scan() {
			fmt.Println(scannerOut.Text())
		}
	}()

	if err != nil {
		log.Fatalf("Failed to run the cmd: %v", err)
	}

	return cmd.Process
}
