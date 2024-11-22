package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "net/http/pprof"

	"github.com/quic-go/quic-go/http3"
	"github.com/valyala/fastrand"
	"golang.org/x/time/rate"
)

func main() {
	// Utilize all available CPU cores
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Start a pprof server for runtime profiling
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	// Get user inputs
	reader := bufio.NewReader(os.Stdin)
	targetURL := getInput(reader, "Enter target URL (e.g., https://target.example.com): ")
	numWorkers := getIntInput(reader, "Enter the number of workers (e.g., 64): ")
	numRequests := getIntInput(reader, "Enter the number of requests per worker (e.g., 10000): ")

	fmt.Println("Starting attack simulation...")

	// Create a rate limiter (configurable rate)
	ratePerSecond := 20
	limiter := rate.NewLimiter(rate.Every(time.Second/time.Duration(ratePerSecond)), ratePerSecond)

	// Worker pool for request execution
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			client, err := createHttp3Client()
			if err != nil {
				log.Printf("Worker %d: Error creating HTTP client: %v", workerID, err)
				return
			}
			executeRequests(ctx, client, targetURL, numRequests, limiter, workerID)
		}(i)
	}

	wg.Wait()
	fmt.Println("Attack simulation complete.")
}

// Get user input as a string
func getInput(reader *bufio.Reader, prompt string) string {
	fmt.Print(prompt)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

// Get user input as an integer
func getIntInput(reader *bufio.Reader, prompt string) int {
	input := getInput(reader, prompt)
	value, err := strconv.Atoi(input)
	if err != nil {
		log.Fatalf("Invalid input: %v", err)
	}
	return value
}

// Execute requests using a rate-limited loop
func executeRequests(ctx context.Context, client *http.Client, targetURL string, numRequests int, limiter *rate.Limiter, workerID int) {
	for i := 0; i < numRequests; i++ {
		if err := limiter.Wait(ctx); err != nil {
			log.Printf("Worker %d: Rate limiter error: %v", workerID, err)
			return
		}

		req, err := http.NewRequest("GET", targetURL, nil)
		if err != nil {
			log.Printf("Worker %d: Error creating request: %v", workerID, err)
			continue
		}

		randomizeRequestHeaders(req)
		resp, err := retryRequest(client, req, workerID, 3)
		if err != nil {
			log.Printf("Worker %d: Error after retries: %v", workerID, err)
			continue
		}
		resp.Body.Close()
		time.Sleep(randomDelay(10*time.Millisecond, 50*time.Millisecond))
	}
}

// Retry requests with exponential backoff
func retryRequest(client *http.Client, req *http.Request, workerID int, retries int) (*http.Response, error) {
	var err error
	var resp *http.Response
	backoff := time.Millisecond * 100
	for i := 0; i < retries; i++ {
		resp, err = client.Do(req)
		if err == nil {
			return resp, nil
		}
		log.Printf("Worker %d: Retry %d failed: %v", workerID, i+1, err)
		time.Sleep(backoff)
		backoff *= 2
	}
	return nil, err
}

// Randomize HTTP headers to avoid detection
func randomizeRequestHeaders(req *http.Request) {
	agents := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/89.0.4389.82 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/14.0.3 Safari/605.1.15",
		"Mozilla/5.0 (X11; Linux x86_64) Gecko/20100101 Firefox/86.0",
	}
	req.Header.Set("User-Agent", agents[fastrand.Uint32n(uint32(len(agents)))])
}

// Create an HTTP/3 client with proper configuration
func createHttp3Client() (*http.Client, error) {
	transport := &http3.RoundTripper{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}, nil
}

// Generate random delays for natural traffic patterns
func randomDelay(min, max time.Duration) time.Duration {
	return min + time.Duration(fastrand.Uint32n(uint32(max-min)))
}
