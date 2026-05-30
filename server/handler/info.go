package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// Set via -ldflags at build time:
//
//	-X goboxd/handler.Version=0.1.0 -X goboxd/handler.GitCommit=abc1234
var (
	Version   = "dev"
	GitCommit = "dev"
)

type buildInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	GoVersion string `json:"go_version"`
}

type nsjailInfo struct {
	Path    string `json:"path"`
	Version string `json:"version"`
}

type defaultLimits struct {
	WallTimeS    int `json:"wall_time_s"`
	MemoryKB     int `json:"memory_kb"`
	MaxProcesses int `json:"max_processes"`
}

type languageInfo struct {
	ID               string        `json:"id"`
	Name             string        `json:"name"`
	Version          string        `json:"version"`
	DefaultRunLimits defaultLimits `json:"default_run_limits"`
}

type globalLimits struct {
	MaxSourceBytes    int `json:"max_source_bytes"`
	MaxTests          int `json:"max_tests"`
	MaxConcurrentJobs int `json:"max_concurrent_jobs"`
}

type statsSnapshot struct {
	InFlightJobs         int64   `json:"in_flight_jobs"`
	JobsTotal            int64   `json:"jobs_total"`
	JobsFailedInternal   int64   `json:"jobs_failed_internal"`
	LastInternalErrorAt  *string `json:"last_internal_error_at"`
	DiskFreeBytesJailDir uint64  `json:"disk_free_bytes_jail_dir"`
}

type infoResponse struct {
	BuildInfo buildInfo      `json:"build_info"`
	Nsjail    nsjailInfo     `json:"nsjail"`
	Languages []languageInfo `json:"languages"`
	Limits    globalLimits   `json:"limits"`
	Stats     statsSnapshot  `json:"stats"`
}

func (h *Handler) info(w http.ResponseWriter, r *http.Request) {
	// Nsjail version
	nsjailVer := probeNsjailVersion(h.cfg.NsjailPath)

	langs := make([]languageInfo, 0, len(h.cfg.Languages))
	for _, lc := range h.cfg.Languages {
		var execPath string
		if lc.CompilationOptions != nil {
			execPath = lc.CompilationOptions.Path
		} else {
			execPath = lc.RuntimeOptions.Path
		}
		versionFlag := lc.VersionFlag
		if versionFlag == "" {
			versionFlag = "--version"
		}
		ver, _ := probeExecutable(execPath, versionFlag)

		name := lc.Language

		dl := defaultLimits{
			WallTimeS:    lc.RuntimeOptions.ResourceLimits.TimeLimit,
			MemoryKB:     lc.RuntimeOptions.ResourceLimits.MemoryLimitMB * 1024,
			MaxProcesses: lc.RuntimeOptions.ResourceLimits.ProcessLimit,
		}
		langs = append(langs, languageInfo{
			ID:               lc.Language,
			Name:             name,
			Version:          ver,
			DefaultRunLimits: dl,
		})
	}

	// Disk free bytes at the jail/sandbox dir
	var diskFree uint64
	var fs syscall.Statfs_t
	if err := syscall.Statfs(h.cfg.SandboxDir, &fs); err == nil {
		diskFree = fs.Bavail * uint64(fs.Bsize)
	}

	// Stats snapshot
	st := h.exec.Stats()
	var lastErrStr *string
	if t := st.LastInternalErrorAt(); t != nil {
		s := t.Format(time.RFC3339)
		lastErrStr = &s
	}
	snap := statsSnapshot{
		InFlightJobs:         st.InFlight.Load(),
		JobsTotal:            st.JobsTotal.Load(),
		JobsFailedInternal:   st.JobsFailedInternal.Load(),
		LastInternalErrorAt:  lastErrStr,
		DiskFreeBytesJailDir: diskFree,
	}

	resp := infoResponse{
		BuildInfo: buildInfo{
			Version:   Version,
			Commit:    GitCommit,
			GoVersion: runtime.Version(),
		},
		Nsjail: nsjailInfo{
			Path:    h.cfg.NsjailPath,
			Version: nsjailVer,
		},
		Languages: langs,
		Limits: globalLimits{
			MaxSourceBytes:    256 * 1024,
			MaxTests:          50,
			MaxConcurrentJobs: 16,
		},
		Stats: snap,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// probeNsjailVersion runs nsjail -h and scrapes the version from stderr.
func probeNsjailVersion(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, path, "-h")
	cmd.Stderr = &buf
	_ = cmd.Run()
	for _, line := range strings.Split(buf.String(), "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "version") || strings.Contains(lower, "nsjail") {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				return trimmed
			}
		}
	}
	return "unknown"
}

// probeExecutable is already defined in handler.go and reused here.
