package main

import (
	"bytes"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

const payload = `{
  "language": "cpp",
  "source": "#include <iostream>\nusing namespace std;\nint main() {\n    int n;\n    cin >> n;\n    long long a = 0, b = 1;\n    for (int i = 0; i < n; i++) {\n        cout << a << \"\\n\";\n        long long c = a + b; a = b; b = c;\n    }\n    return 0;\n}",
  "tests": [
    {
      "stdin": "5\n",
      "expected_stdout": "0\n1\n1\n2\n3\n"
    }
  ]
}`

func main() {
	urlFlag := flag.String("url", "http://localhost:8000/run", "Target URL")
	concurrencyFlag := flag.Int("c", 10, "Concurrency level (number of concurrent clients)")
	requestsFlag := flag.Int("n", 100, "Total number of requests to send")
	flag.Parse()

	url := *urlFlag
	concurrency := *concurrencyFlag
	totalRequests := *requestsFlag

	if concurrency < 1 {
		concurrency = 1
	}
	if totalRequests < 1 {
		totalRequests = 1
	}
	if totalRequests < concurrency {
		totalRequests = concurrency
	}

	fmt.Printf("Starting load test on %s\n", url)
	fmt.Printf("Concurrency: %d, Total Requests: %d\n", concurrency, totalRequests)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	latencies := make([]time.Duration, totalRequests)
	var latenciesMu sync.Mutex
	var wg sync.WaitGroup

	reqChan := make(chan int, totalRequests)
	for i := 0; i < totalRequests; i++ {
		reqChan <- i
	}
	close(reqChan)

	successCount := 0
	failureCount := 0
	var statsMu sync.Mutex

	startTime := time.Now()

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for reqIdx := range reqChan {
				reqStart := time.Now()
				resp, err := client.Post(url, "application/json", bytes.NewBufferString(payload))
				duration := time.Since(reqStart)

				statsMu.Lock()
				if err != nil {
					failureCount++
				} else {
					if resp.StatusCode == http.StatusOK {
						successCount++
					} else {
						failureCount++
					}
					resp.Body.Close()
				}
				statsMu.Unlock()

				latenciesMu.Lock()
				latencies[reqIdx] = duration
				latenciesMu.Unlock()
			}
		}()
	}

	wg.Wait()
	totalDuration := time.Since(startTime)

	// Filter zero durations in case of early errors or mismatched request counts (should not happen, but safe)
	var validLatencies []time.Duration
	for _, l := range latencies {
		if l > 0 {
			validLatencies = append(validLatencies, l)
		}
	}

	if len(validLatencies) == 0 {
		fmt.Println("No successful or valid request latencies recorded.")
		os.Exit(1)
	}

	// Sort latencies to compute percentiles
	sort.Slice(validLatencies, func(i, j int) bool {
		return validLatencies[i] < validLatencies[j]
	})

	getPercentile := func(p float64) time.Duration {
		idx := int(float64(len(validLatencies)) * p)
		if idx >= len(validLatencies) {
			idx = len(validLatencies) - 1
		}
		if idx < 0 {
			idx = 0
		}
		return validLatencies[idx]
	}

	p50 := getPercentile(0.50)
	p95 := getPercentile(0.95)
	p99 := getPercentile(0.99)

	rps := float64(successCount+failureCount) / totalDuration.Seconds()

	fmt.Println("\n================ Benchmarks Result ================")
	fmt.Printf("Total Time:      %.3fs\n", totalDuration.Seconds())
	fmt.Printf("Requests/sec:    %.2f\n", rps)
	fmt.Printf("Successes:       %d\n", successCount)
	fmt.Printf("Failures:        %d\n", failureCount)
	fmt.Printf("p50 latency:     %v\n", p50)
	fmt.Printf("p95 latency:     %v\n", p95)
	fmt.Printf("p99 latency:     %v\n", p99)
	fmt.Println("===================================================")
}
