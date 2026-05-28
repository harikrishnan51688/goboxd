# goboxd Load Testing Benchmarks

This document records the load testing benchmarks of the `goboxd` sandboxed execution server, run inside a clean Docker environment on an Apple Silicon macOS host.

## Environment Details
- **OS**: macOS 15.x (Darwin arm64)
- **CPU**: Apple M-series
- **Docker Resources**: Default resources allocated to Docker Desktop (virtualized Linux kernel virtual machine)
- **Target Language**: Python 3 (`py3`) Hello World (trivial print)
- **Payload**:
  ```json
  {
    "language": "py3",
    "source": "print('Hello from Python 3!')",
    "tests": [
      {
        "stdin": "",
        "expected_stdout": "Hello from Python 3!\n"
      }
    ]
  }
  ```

---

## Benchmark Results

Below are the results for 1, 10, 50, and 100 concurrent clients executing requests concurrently.

| Concurrent Clients | Total Requests | Total Time (s) | Requests / Sec | p50 Latency (ms) | p95 Latency (ms) | p99 Latency (ms) | Success Rate |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **1** | 100 | 1.076 | 92.97 | 10.497 | 13.037 | 16.578 | 100% |
| **10** | 100 | 0.317 | 315.49 | 23.571 | 88.621 | 95.634 | 100% |
| **50** | 200 | 0.486 | 411.74 | 112.271 | 142.369 | 153.765 | 100% |
| **100** | 400 | 0.946 | 422.89 | 225.599 | 256.585 | 273.001 | 100% |

---

## Observation & Metrics Visibility

1. **Queueing Behavior**:
   As concurrency exceeds the available CPU resources, requests queue up gracefully in Go's internal HTTP handler queue rather than returning failures or causing `nsjail` resource exhaustion.
2. **Captured Metrics**:
   Per-request CPU usage (user + system) and wall-clock times are captured and visible in:
   - **Response Headers**:
     - `X-Queue-Time-Ms`: Time spent in the concurrency semaphore queue before processing.
     - `X-Wall-Time-Ms`: Processing duration (compilation + execution).
     - `X-CPU-Time-Ms`: Total CPU time consumed by the sandboxed process.
   - **Application Logs**:
     - Structured log entries containing metrics are output for every run:
       ```
       [METRIC] language=py3 tests=1 queue_time_ms=184 wall_time_ms=15 cpu_time_ms=14 status=ok
       ```
