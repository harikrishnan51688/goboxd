# goboxd Load Testing Benchmarks

This document records the load testing benchmarks of the `goboxd` sandboxed execution server, run inside a clean Docker environment on an Apple Silicon macOS host.

## Environment Details
- **OS**: macOS 15.x (Darwin arm64)
- **CPU**: Apple M-series
- **Docker Resources**: Default resources allocated to Docker Desktop (virtualized Linux kernel virtual machine)

### Payloads
```Test using one Interpretered language (Python 3) and one Compiled language (C) to demonstrate the performance in a sandboxed environment.```

#### Python 3 (`py3`) Hello World
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

#### C (`c`) Hello World
```json
{
  "language": "c",
  "source": "#include <stdio.h>\nint main() {\n    printf(\"Hello from C!\\n\");\n    return 0;\n}",
  "tests": [
    {
      "stdin": "",
      "expected_stdout": "Hello from C!\n"
    }
  ]
}
```

---

## Benchmark Results

Below are the results for 1, 10, 50, and 100 concurrent clients executing requests concurrently.

### Python 3 (`py3`) Benchmarks
| Concurrent Clients | Total Requests | Total Time (s) | Requests / Sec | p50 Latency (ms) | p95 Latency (ms) | p99 Latency (ms) | Success Rate |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **1** | 100 | 0.958 | 104.40 | 8.653 | 13.239 | 48.251 | 100% |
| **10** | 100 | 0.251 | 398.11 | 20.916 | 46.310 | 53.420 | 100% |
| **50** | 200 | 0.478 | 418.58 | 101.428 | 121.404 | 127.780 | 100% |
| **100** | 400 | 0.915 | 436.98 | 212.923 | 240.149 | 245.450 | 100% |

### C (`c`) Benchmarks
| Concurrent Clients | Total Requests | Total Time (s) | Requests / Sec | p50 Latency (ms) | p95 Latency (ms) | p99 Latency (ms) | Success Rate |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **1** | 100 | 1.792 | 55.79 | 17.209 | 20.661 | 51.440 | 100% |
| **10** | 100 | 0.430 | 232.81 | 38.526 | 59.028 | 85.089 | 100% |
| **50** | 200 | 0.874 | 228.90 | 199.251 | 233.552 | 256.269 | 100% |
| **100** | 400 | 1.774 | 225.49 | 422.839 | 458.863 | 473.200 | 100% |

---

## Observation & Metrics Visibility

1. **Queueing Behavior**:
   As concurrency exceeds the available CPU resources, requests queue up gracefully in Go's internal HTTP handler queue rather than returning failures or causing `nsjail` resource exhaustion.
2. **Interpreted vs. Compiled Performance**:
   For C, each run includes a compilation phase (`gcc -o a.out solution.c`) inside the sandbox before execution. This results in lower throughput (RPS) and higher latencies compared to Python 3, which is interpreted directly.
3. **Captured Metrics**:
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
