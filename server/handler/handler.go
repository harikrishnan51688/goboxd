package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	"goboxd/config"
	"goboxd/executor"
	pb "goboxd/proto"

	"google.golang.org/protobuf/encoding/protojson"
)

// Handler holds the HTTP handler state.
type Handler struct {
	cfg  *config.Config
	exec *executor.Executor
}

// New creates a Handler backed by the given Config.
func New(cfg *config.Config) *Handler {
	return &Handler{
		cfg:  cfg,
		exec: executor.New(cfg),
	}
}

// Handle routes GET / to health check, GET /readyz to readiness check, and POST /run to judge.
func (h *Handler) Handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if r.URL.Path == "/readyz" {
			h.readyz(w, r)
		} else if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		} else {
			http.Error(w, "not found", http.StatusNotFound)
		}

	case http.MethodPost:
		if r.URL.Path == "/run" {
			h.judge(w, r)
		} else {
			http.Error(w, "not found", http.StatusNotFound)
		}

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

func writeErrorResponse(w http.ResponseWriter, code string, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	resp := ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: msg,
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) judge(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse using protojson with DiscardUnknown enabled
	var req pb.RunRequest
	unmarshalOpts := protojson.UnmarshalOptions{
		DiscardUnknown: true,
	}
	if err := unmarshalOpts.Unmarshal(body, &req); err != nil {
		writeErrorResponse(w, "bad_json", "invalid JSON: "+err.Error())
		return
	}

	// 1. Language validation
	if req.Language == "" {
		writeErrorResponse(w, "missing_language", "language is required")
		return
	}
	lang, err := h.cfg.Lookup(req.Language)
	if err != nil {
		writeErrorResponse(w, "unknown_language", fmt.Sprintf("unknown language: %s", req.Language))
		return
	}

	// 2. Source validation
	if req.Source == "" {
		writeErrorResponse(w, "missing_source", "source code is required")
		return
	}
	// Default max size 256 KiB (262,144 bytes)
	if len(req.Source) > 256*1024 {
		writeErrorResponse(w, "source_too_large", "source code exceeds maximum size of 256 KiB")
		return
	}
	if !utf8.ValidString(req.Source) {
		writeErrorResponse(w, "invalid_encoding", "source code must be valid UTF-8")
		return
	}

	// 3. Filename validation
	needsFilenames := lang.Filename == "TAKE_FROM_REQUEST" || lang.BinaryFilename == "TAKE_FROM_REQUEST"
	if needsFilenames {
		if req.SourceFilename == "" {
			writeErrorResponse(w, "missing_filename", "source_filename is required for this language")
			return
		}
		if req.ArtifactFilename == "" {
			writeErrorResponse(w, "missing_filename", "artifact_filename is required for this language")
			return
		}
	}

	validateFilename := func(filename, fieldName string) bool {
		if filename == "" {
			return true
		}
		if strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
			writeErrorResponse(w, "invalid_filename", fmt.Sprintf("%s must be a single path component", fieldName))
			return false
		}
		if strings.HasPrefix(filename, ".") {
			writeErrorResponse(w, "invalid_filename", fmt.Sprintf("%s must not have a leading dot", fieldName))
			return false
		}
		if len(filename) > 255 {
			writeErrorResponse(w, "invalid_filename", fmt.Sprintf("%s exceeds maximum length of 255 characters", fieldName))
			return false
		}
		return true
	}

	if !validateFilename(req.SourceFilename, "source_filename") {
		return
	}
	if !validateFilename(req.ArtifactFilename, "artifact_filename") {
		return
	}

	// 4. Flags validation
	isFlagAllowed := func(allowedFlags []string, flag string) bool {
		for _, f := range allowedFlags {
			if f == flag {
				return true
			}
		}
		return false
	}

	if req.Build != nil {
		for _, flag := range req.Build.Flags {
			if !isFlagAllowed(lang.AllowedBuildFlags, flag) {
				writeErrorResponse(w, "disallowed_flag", fmt.Sprintf("flag %q is not allowed for build in language %s", flag, req.Language))
				return
			}
		}
	}

	if req.Run != nil {
		for _, flag := range req.Run.Flags {
			if !isFlagAllowed(lang.AllowedRunFlags, flag) {
				writeErrorResponse(w, "disallowed_flag", fmt.Sprintf("flag %q is not allowed for run in language %s", flag, req.Language))
				return
			}
		}
	}

	// 5. Tests validation
	if len(req.Tests) == 0 {
		writeErrorResponse(w, "missing_tests", "at least one test case is required")
		return
	}

	log.Printf("judge: lang=%s tests=%d", req.Language, len(req.Tests))

	// Execute the run
	resp, err := h.exec.Run(&req)
	if err != nil {
		log.Printf("Sandbox setup / system error: %v", err)
		http.Error(w, "internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Serialize output with protojson (UseProtoNames ensures snake_case JSON keys)
	marshalOpts := protojson.MarshalOptions{
		UseProtoNames: true,
	}
	respBytes, err := marshalOpts.Marshal(resp)
	if err != nil {
		log.Printf("Proto JSON marshal error: %v", err)
		http.Error(w, "proto marshal error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(respBytes)
}

type ComponentStatus struct {
	OK      bool   `json:"ok"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

type ReadinessResponse struct {
	Status    string                     `json:"status"`
	Nsjail    ComponentStatus            `json:"nsjail"`
	Languages map[string]ComponentStatus `json:"languages"`
}

func (h *Handler) readyz(w http.ResponseWriter, r *http.Request) {
	overallOK := true

	// 1. Probe nsjail
	nsjailOK := true
	nsjailVersion := "3.4"
	nsjailErrStr := ""

	nsjailInfo, err := os.Stat(h.cfg.NsjailPath)
	if err != nil {
		nsjailOK = false
		nsjailErrStr = fmt.Sprintf("nsjail not found at %s", h.cfg.NsjailPath)
	} else if nsjailInfo.IsDir() {
		nsjailOK = false
		nsjailErrStr = fmt.Sprintf("nsjail path %s is a directory", h.cfg.NsjailPath)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, h.cfg.NsjailPath, "-h")
		if err := cmd.Run(); err != nil {
			if _, ok := err.(*exec.ExitError); !ok {
				nsjailOK = false
				nsjailErrStr = fmt.Sprintf("nsjail execution failed: %v", err)
			}
		}
	}

	if !nsjailOK {
		overallOK = false
	}

	nsjailStatus := ComponentStatus{
		OK: nsjailOK,
	}
	if nsjailOK {
		nsjailStatus.Version = nsjailVersion
	} else {
		nsjailStatus.Error = nsjailErrStr
	}

	// 2. Probe configured languages
	languagesStatus := make(map[string]ComponentStatus)

	for _, lang := range h.cfg.Languages {
		var path string
		if lang.CompilationOptions != nil {
			path = lang.CompilationOptions.Path
		} else {
			path = lang.RuntimeOptions.Path
		}

		ver, err := probeExecutable(path)
		if err != nil {
			overallOK = false
			languagesStatus[lang.Language] = ComponentStatus{
				OK:    false,
				Error: err.Error(),
			}
		} else {
			languagesStatus[lang.Language] = ComponentStatus{
				OK:      true,
				Version: ver,
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")

	statusStr := "ok"
	if !overallOK {
		statusStr = "degraded"
		w.WriteHeader(http.StatusServiceUnavailable) // 503
	} else {
		w.WriteHeader(http.StatusOK) // 200
	}

	resp := ReadinessResponse{
		Status:    statusStr,
		Nsjail:    nsjailStatus,
		Languages: languagesStatus,
	}

	_ = json.NewEncoder(w).Encode(resp)
}

func probeExecutable(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("not found at %s", path)
	}
	if info.IsDir() {
		return "", fmt.Errorf("path %s is a directory", path)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "--version")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("execution failed: %v (stderr: %q)", err, stderr.String())
	}

	outStr := stdout.String()
	if outStr == "" {
		outStr = stderr.String()
	}

	lines := strings.Split(strings.TrimSpace(outStr), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return "unknown version", nil
	}

	return strings.TrimSpace(lines[0]), nil
}
