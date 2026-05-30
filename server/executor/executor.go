package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"goboxd/config"
	pb "goboxd/proto"
	"goboxd/stats"
)

// maxOutputBytes is the per-stream (stdout / stderr) cap for one sandbox run.
// Output beyond this limit is silently discarded and a truncation marker is appended.
const maxOutputBytes = 1 << 20 // 1 MiB

const truncationMarker = "\n[... output truncated ...]\n"

// cappedWriter wraps a bytes.Buffer and stops accepting new data once the cap
// is reached, then appends a truncation marker on the first overflow.
type cappedWriter struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	cap   int
	trunc bool // true once we have overflowed
}

func newCappedWriter(cap int) *cappedWriter {
	return &cappedWriter{cap: cap}
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.trunc {
		// Already truncated — accept the call (return n so the child doesn't
		// get a broken-pipe) but discard the bytes.
		return len(p), nil
	}
	remain := c.cap - c.buf.Len()
	if remain <= 0 {
		c.trunc = true
		c.buf.WriteString(truncationMarker)
		return len(p), nil
	}
	if len(p) > remain {
		c.buf.Write(p[:remain])
		c.trunc = true
		c.buf.WriteString(truncationMarker)
	} else {
		c.buf.Write(p)
	}
	return len(p), nil
}

func (c *cappedWriter) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

// Executor runs code in nsjail using the provided Config.
type Executor struct {
	cfg        *config.Config
	stats      *stats.Stats
	jobCounter int64
}

func New(cfg *config.Config) *Executor {
	// Sweep stale orphaned jail directories concurrently on startup
	go cleanOrphanedJails(10 * time.Minute)
	return &Executor{cfg: cfg, stats: &stats.Stats{}}
}

func cleanOrphanedJails(maxAge time.Duration) {
	tempDir := os.TempDir()
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		log.Printf("Failed to read temp dir %s for orphan cleanup: %v", tempDir, err)
		return
	}

	cleaned := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if !strings.HasPrefix(entry.Name(), "goboxd-") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if time.Since(info.ModTime()) > maxAge {
			path := filepath.Join(tempDir, entry.Name())
			if err := os.RemoveAll(path); err == nil {
				cleaned++
			}
		}
	}
	if cleaned > 0 {
		log.Printf("Startup sweep: Cleaned up %d orphaned stale jail directories from %s", cleaned, tempDir)
	}
}

// sandboxResult is the internal result of a single nsjail invocation.
type sandboxResult struct {
	Status string
	Stdout string
	Stderr string
}

// Run is the public entry point — it tracks stats and delegates to run.
func (e *Executor) Run(req *pb.RunRequest) (*pb.RunResponse, int64, error) {
	e.stats.InFlight.Add(1)
	defer e.stats.InFlight.Add(-1)
	e.stats.JobsTotal.Add(1)

	resp, cpuMs, err := e.run(req)
	if err != nil {
		e.stats.RecordInternalError()
	}
	return resp, cpuMs, err
}

func (e *Executor) Stats() *stats.Stats {
	return e.stats
}

// run executes a RunRequest and returns a RunResponse, along with CPU time in milliseconds.
func (e *Executor) run(req *pb.RunRequest) (*pb.RunResponse, int64, error) {
	lang, err := e.cfg.Lookup(req.Language)
	if err != nil {
		return nil, 0, fmt.Errorf("lookup language: %w", err)
	}

	// Create an isolated temp working directory on the host.
	// nsjail will bind-mount it into the chroot as /work.
	workDir, err := os.MkdirTemp("", "goboxd-*")
	if err != nil {
		return nil, 0, fmt.Errorf("mkdirtemp: %w", err)
	}
	defer os.RemoveAll(workDir)

	// Determine source and binary filenames
	srcFile, binFile, err := resolveFilenames(lang, req)
	if err != nil {
		return nil, 0, fmt.Errorf("resolve filenames: %w", err)
	}

	// Write source code into the working directory
	srcPath := filepath.Join(workDir, srcFile)
	if err := os.WriteFile(srcPath, []byte(req.Source), 0644); err != nil {
		return nil, 0, fmt.Errorf("write source: %w", err)
	}

	templateVars := map[string]string{
		"FILENAME":        srcFile,
		"BINARY_FILENAME": binFile,
		"EXTRA_ARGS":      "",
	}

	resp := &pb.RunResponse{}
	var totalCpuTimeMs int64

	// Compilation step (skipped for interpreted languages)
	if lang.CompilationOptions != nil {
		buildOpts := overrideOptions(lang.CompilationOptions, req.Build)
		var buildFlags []string
		if req.Build != nil {
			buildFlags = req.Build.Flags
		}
		r, compDuration, _, compCpuMs := e.runInSandbox(workDir, buildOpts, templateVars, "", buildFlags)
		totalCpuTimeMs += compCpuMs

		buildStatus := "ok"
		if r.Status != "ok" {
			buildStatus = "failed"
		}

		resp.Build = &pb.BuildResult{
			Status:     buildStatus,
			Stdout:     r.Stdout,
			Stderr:     r.Stderr,
			DurationMs: int32(compDuration),
		}

		if r.Status != "ok" {
			resp.Status = "compilation_error"
			return resp, totalCpuTimeMs, nil
		}
	}

	// Test case execution
	runtimeOpts := overrideOptions(&lang.RuntimeOptions, req.Run)
	var runFlags []string
	if req.Run != nil {
		runFlags = req.Run.Flags
	}

	overallStatus := "ok"
	for _, tc := range req.Tests {
		tcr, cpuMs := e.runTestCase(workDir, lang, runtimeOpts, templateVars, tc, runFlags)
		totalCpuTimeMs += cpuMs
		resp.Tests = append(resp.Tests, tcr)
		if tcr.Status != "ok" && overallStatus == "ok" {
			overallStatus = tcr.Status
		}
	}
	resp.Status = overallStatus

	return resp, totalCpuTimeMs, nil
}

// runTestCase runs one test case and compares output to expected.
func (e *Executor) runTestCase(
	workDir string,
	lang *config.LanguageConfig,
	opts *config.ExecutionOptions,
	templateVars map[string]string,
	tc *pb.TestCase,
	flags []string,
) (*pb.TestResult, int64) {
	r, durationMs, memoryPeakKB, cpuMs := e.runInSandbox(workDir, opts, templateVars, tc.Stdin, flags)

	tcr := &pb.TestResult{
		Stdout:       r.Stdout,
		Stderr:       r.Stderr,
		DurationMs:   int32(durationMs),
		MemoryPeakKb: int32(memoryPeakKB),
	}

	switch r.Status {
	case "ok":
		if r.Stdout == tc.ExpectedStdout {
			tcr.Status = "ok"
		} else {
			tcr.Status = "wrong_output"
		}
	default:
		tcr.Status = r.Status
	}
	return tcr, cpuMs
}

// runInSandbox builds and runs an nsjail command, returning a sandboxResult, duration in ms, peak memory in kb, and CPU time in ms.
func (e *Executor) runInSandbox(
	workDir string,
	opts *config.ExecutionOptions,
	templateVars map[string]string,
	stdin string,
	flags []string,
) (*sandboxResult, int64, int64, int64) {
	// Build nsjail arguments
	// Generate a unique UID/GID for this run to prevent collisions under load.
	// We use a pool of 100,000 UIDs starting from 100000.
	runID := atomic.AddInt64(&e.jobCounter, 1)
	uid := 100000 + (runID % 100000)

	args := []string{
		"-Mo",
		"--chroot", e.cfg.SandboxDir,
		"--user", fmt.Sprintf("%d", uid),
		"--group", fmt.Sprintf("%d", uid),
		"--time_limit", fmt.Sprintf("%d", opts.ResourceLimits.TimeLimit),
		"--rlimit_as", fmt.Sprintf("%d", opts.ResourceLimits.MemoryLimitMB),
		"--max_cpus", "1",
		"--cwd", "/work",
		// Bind-mount the temp dir as /work inside the chroot
		"-B", fmt.Sprintf("%s:/work", workDir),
	}

	if opts.ResourceLimits.ProcessLimit > 0 {
		args = append(args, "--rlimit_nproc", fmt.Sprintf("%d", opts.ResourceLimits.ProcessLimit))
	}

	// Append default nsjail flags from config (e.g. --disable_clone_newnet)
	args = append(args, e.cfg.DefaultNsjailArgs...)

	// Separator before the sandboxed program
	args = append(args, "--")
	args = append(args, opts.Path)
	args = append(args, config.ExpandArgsWithFlags(opts.Args, templateVars, flags)...)

	// Run with a deadline slightly above the sandbox time_limit
	timeout := time.Duration(opts.ResourceLimits.TimeLimit+2) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.cfg.NsjailPath, args...)

	stdout := newCappedWriter(maxOutputBytes)
	stderr := newCappedWriter(maxOutputBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	// Pipe fd 3 → nsjail's own log so we can read it after the run
	logR, logW, pipeErr := os.Pipe()
	if pipeErr == nil {
		cmd.ExtraFiles = []*os.File{logW} // fd 3
	}

	startTime := time.Now()
	runErr := cmd.Run()
	durationMs := time.Since(startTime).Milliseconds()

	// Close write end so ReadAll doesn't block, then drain the log
	var nsjailLog string
	if pipeErr == nil {
		logW.Close()
		logBytes, _ := io.ReadAll(logR)
		logR.Close()
		nsjailLog = string(logBytes)
	}

	var memoryPeakKB int64 = 0
	if cmd.ProcessState != nil && cmd.ProcessState.SysUsage() != nil {
		if rusage, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage); ok {
			memoryPeakKB = rusage.Maxrss
		}
	}

	var cpuTimeMs int64 = 0
	if cmd.ProcessState != nil {
		cpuTimeMs = (cmd.ProcessState.UserTime() + cmd.ProcessState.SystemTime()).Milliseconds()
	}

	stderrContent := stderr.String()
	if runErr != nil && stderrContent == "" {
		stderrContent = nsjailLog
	}

	if ctx.Err() == context.DeadlineExceeded {
		return &sandboxResult{
			Status: "time_limit_exceeded",
			Stdout: stdout.String(),
			Stderr: "time limit exceeded (host timeout)",
		}, durationMs, memoryPeakKB, cpuTimeMs
	}

	if runErr != nil {
		combinedStderr := nsjailLog + stderr.String()
		return &sandboxResult{
			Status: classifyFailure(combinedStderr),
			Stdout: stdout.String(),
			Stderr: stderrContent,
		}, durationMs, memoryPeakKB, cpuTimeMs
	}

	return &sandboxResult{
		Status: "ok",
		Stdout: stdout.String(),
		Stderr: stderr.String(), // keep it strictly clean for successful runs
	}, durationMs, memoryPeakKB, cpuTimeMs
}

// classifyFailure inspects nsjail's stderr log to pick the right Status string.
func classifyFailure(nsjailLog string) string {
	lower := strings.ToLower(nsjailLog)
	switch {
	case strings.Contains(lower, "time limit") || strings.Contains(lower, "timelimit"):
		return "time_limit_exceeded"
	case strings.Contains(lower, "memory") || strings.Contains(lower, "oom"):
		return "memory_limit_exceeded"
	default:
		return "runtime_error"
	}
}

// resolveFilenames determines the actual source and binary filenames.
func resolveFilenames(lang *config.LanguageConfig, req *pb.RunRequest) (string, string, error) {
	src := req.SourceFilename
	bin := req.ArtifactFilename

	if src == "" {
		src = lang.Filename
	}
	if bin == "" {
		bin = lang.BinaryFilename
	}

	if src == "TAKE_FROM_REQUEST" {
		className, err := extractJavaClassName(req.Source)
		if err != nil {
			return "", "", fmt.Errorf("java filename: %w", err)
		}
		src = className + ".java"
		bin = className
	}
	if bin == "" {
		bin = src
	}
	return src, bin, nil
}

var javaClassRe = regexp.MustCompile(`public\s+class\s+(\w+)`)

func extractJavaClassName(code string) (string, error) {
	m := javaClassRe.FindStringSubmatch(code)
	if m == nil {
		return "", fmt.Errorf("no public class found in Java source")
	}
	return m[1], nil
}

func overrideOptions(base *config.ExecutionOptions, stepReq *pb.StepConfig) *config.ExecutionOptions {
	if base == nil {
		return nil
	}
	opts := &config.ExecutionOptions{
		Path: base.Path,
		Args: make([]string, len(base.Args)),
		ResourceLimits: config.ResourceLimits{
			TimeLimit:     base.ResourceLimits.TimeLimit,
			ProcessLimit:  base.ResourceLimits.ProcessLimit,
			MemoryLimitMB: base.ResourceLimits.MemoryLimitMB,
		},
	}
	copy(opts.Args, base.Args)

	if stepReq == nil {
		return opts
	}

	if stepReq.Limits != nil {
		if stepReq.Limits.WallTimeS > 0 {
			opts.ResourceLimits.TimeLimit = int(stepReq.Limits.WallTimeS)
		}
		if stepReq.Limits.MaxProcesses > 0 {
			opts.ResourceLimits.ProcessLimit = int(stepReq.Limits.MaxProcesses)
		}
		if stepReq.Limits.MemoryKb > 0 {
			// Convert KB to MB (taking ceiling)
			opts.ResourceLimits.MemoryLimitMB = int((stepReq.Limits.MemoryKb + 1023) / 1024)
		}
	}

	return opts
}
