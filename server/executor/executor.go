package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"goboxd/config"
	pb "goboxd/proto"
)

// Executor runs code in nsjail using the provided Config.
type Executor struct {
	cfg *config.Config
}

func New(cfg *config.Config) *Executor {
	return &Executor{cfg: cfg}
}

// sandboxResult is the internal result of a single nsjail invocation.
type sandboxResult struct {
	Status pb.Status
	Stdout string
	Stderr string
}

// Run executes a JudgeRequest and returns a JudgeResponse.
func (e *Executor) Run(req *pb.JudgeRequest) *pb.JudgeResponse {
	lang, err := e.cfg.Lookup(req.Language)
	if err != nil {
		return errorResponse(fmt.Sprintf("unknown language: %v", err))
	}

	// Create an isolated temp working directory on the host.
	// nsjail will bind-mount it into the chroot as /work.
	workDir, err := os.MkdirTemp("", "goboxd-*")
	if err != nil {
		return errorResponse(fmt.Sprintf("mkdirtemp: %v", err))
	}
	defer os.RemoveAll(workDir)

	// Determine source and binary filenames
	srcFile, binFile, err := resolveFilenames(lang, req)
	if err != nil {
		return errorResponse(err.Error())
	}

	// Write source code into the working directory
	srcPath := filepath.Join(workDir, srcFile)
	if err := os.WriteFile(srcPath, []byte(req.Code.FullCode), 0644); err != nil {
		return errorResponse(fmt.Sprintf("write source: %v", err))
	}

	templateVars := map[string]string{
		"FILENAME":        srcFile,
		"BINARY_FILENAME": binFile,
		"EXTRA_ARGS":      "",
	}

	resp := &pb.JudgeResponse{}

	// ── Compilation step (skipped for interpreted languages) ──────────────
	if lang.CompilationOptions != nil {
		r := e.runInSandbox(workDir, lang.CompilationOptions, templateVars, "")
		resp.CompilationResult = &pb.CompilationResult{
			Status: r.Status,
			Output: r.Stdout,
			Error:  r.Stderr,
		}
		if r.Status != pb.Status_OK {
			resp.OverallStatus = pb.Status_COMPILATION_ERROR
			return resp
		}
	} else {
		resp.CompilationResult = &pb.CompilationResult{Status: pb.Status_OK}
	}

	// ── Test case execution ───────────────────────────────────────────────
	overallOK := true
	for _, tc := range req.Testcases {
		tcr := e.runTestCase(workDir, lang, templateVars, tc)
		resp.TestCaseResults = append(resp.TestCaseResults, tcr)
		if tcr.Status != pb.Status_OK {
			overallOK = false
		}
	}

	if overallOK {
		resp.OverallStatus = pb.Status_OK
	} else {
		resp.OverallStatus = pb.Status_ERROR
	}
	return resp
}

// runTestCase runs one test case and compares output to expected.
func (e *Executor) runTestCase(
	workDir string,
	lang *config.LanguageConfig,
	templateVars map[string]string,
	tc *pb.TestCase,
) *pb.TestCaseResult {
	r := e.runInSandbox(workDir, &lang.RuntimeOptions, templateVars, tc.Input)

	tcr := &pb.TestCaseResult{
		ExpectedOutput: tc.Output,
		ActualOutput:   r.Stdout,
	}

	switch r.Status {
	case pb.Status_OK:
		if r.Stdout == tc.Output {
			tcr.Status = pb.Status_OK
		} else {
			tcr.Status = pb.Status_ERROR
			tcr.Error = "wrong answer"
		}
	default:
		tcr.Status = r.Status
		tcr.Error = r.Stderr
	}
	return tcr
}

// runInSandbox builds and runs an nsjail command, returning a sandboxResult.
func (e *Executor) runInSandbox(
	workDir string,
	opts *config.ExecutionOptions,
	templateVars map[string]string,
	stdin string,
) *sandboxResult {
	// ── Build nsjail arguments ────────────────────────────────────────────
	args := []string{
		"-Mo",
		"--chroot", e.cfg.SandboxDir,
		"--user", "99999",
		"--group", "99999",
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
	args = append(args, config.ExpandArgs(opts.Args, templateVars)...)

	// ── Run with a deadline slightly above the sandbox time_limit ─────────
	timeout := time.Duration(opts.ResourceLimits.TimeLimit+2) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.cfg.NsjailPath, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	// Pipe fd 3 → nsjail's own log so we can read it after the run
	logR, logW, pipeErr := os.Pipe()
	if pipeErr == nil {
		cmd.ExtraFiles = []*os.File{logW} // fd 3
	}

	runErr := cmd.Run()

	// Close write end so ReadAll doesn't block, then drain the log
	var nsjailLog string
	if pipeErr == nil {
		logW.Close()
		logBytes, _ := io.ReadAll(logR)
		logR.Close()
		nsjailLog = string(logBytes)
	}

	if ctx.Err() == context.DeadlineExceeded {
		return &sandboxResult{Status: pb.Status_TLE, Stderr: "time limit exceeded (host timeout)"}
	}

	if runErr != nil {
		return &sandboxResult{
			Status: classifyFailure(nsjailLog + stderr.String()),
			Stdout: stdout.String(),
			Stderr: nsjailLog,
		}
	}

	return &sandboxResult{
		Status: pb.Status_OK,
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
}

// classifyFailure inspects nsjail's stderr log to pick the right Status.
func classifyFailure(nsjailLog string) pb.Status {
	lower := strings.ToLower(nsjailLog)
	switch {
	case strings.Contains(lower, "time limit") || strings.Contains(lower, "timelimit"):
		return pb.Status_TLE
	case strings.Contains(lower, "memory") || strings.Contains(lower, "oom"):
		return pb.Status_MLE
	default:
		return pb.Status_RUNTIME_ERROR
	}
}

// resolveFilenames determines the actual source and binary filenames.
// For Java, the public class name is extracted from the source code.
func resolveFilenames(lang *config.LanguageConfig, req *pb.JudgeRequest) (string, string, error) {
	src := lang.Filename
	bin := lang.BinaryFilename

	if src == "TAKE_FROM_REQUEST" {
		className, err := extractJavaClassName(req.Code.FullCode)
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

func errorResponse(msg string) *pb.JudgeResponse {
	return &pb.JudgeResponse{
		OverallStatus: pb.Status_ERROR,
		CompilationResult: &pb.CompilationResult{
			Status: pb.Status_ERROR,
			Error:  msg,
		},
	}
}
