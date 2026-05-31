<div align="center">

# goboxd

**A Go HTTP service for executing untrusted code in isolated sandboxes.**

</div>

---

## Overview

goboxd is an HTTP service written in Go that compiles and runs untrusted code inside isolated sandboxes and returns the result. Optional test cases can be supplied to assert behaviour against expected output. It is built for safe execution of code across many languages, with strict isolation, bounded concurrency, and a plug and play language registry.

## Features

- Plug and play language registry driven by YAML
- Process isolation using Linux namespaces and cgroups
- Bounded concurrency with request queuing
- Fully containerised for local development and deployment
- Per request resource limits for time, memory, and processes
- Liveness and readiness probes for orchestration

## Getting started

### Prerequisites

- Docker with Compose v2

No Go toolchain or system dependencies are required on the host. Everything runs in containers.

### Installation

```sh
git clone -b team/meow https://github.com/harikrishnan51688/goboxd.git
cd goboxd
make build
make up
```

### Usage

```sh
make build        # build the docker-compose image
make up           # start the service in background on port 8000
make down         # stop all containers
make test         # run API integration tests
make load         # run load tests
```


## Project structure

```
.
├── docs/         # Documentation (benchmarks, etc.)
├── load/         # Load testing tools
├── scripts/      # Installer scripts for sandboxed environments
├── server/       # Main backend source tree
│   ├── config/   # YAML-driven language configurations and limits
│   ├── executor/ # Sandbox execution and nsjail command wrappers
│   ├── handler/  # HTTP request controllers (/run, /readyz, /info)
│   ├── proto/    # Protocol Buffers schema
│   ├── stats/    # Operational counters & health metrics
│   └── main.go   # Server entry point
├── tests/        # API integration test runner and inputs
├── Makefile      # Build, execution, and testing shortcuts
└── docker-compose.yml # Containerized services definition
```
