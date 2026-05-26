package handler

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"goboxd/config"
	"goboxd/executor"
	pb "goboxd/proto"

	"google.golang.org/protobuf/encoding/prototext"
)

// messageWrapper matches the JSON envelope { "message": "..." }
type messageWrapper struct {
	Message string `json:"message"`
}

// Handler holds the HTTP handler state.
type Handler struct {
	exec *executor.Executor
}

// New creates a Handler backed by the given Config.
func New(cfg *config.Config) *Handler {
	return &Handler{exec: executor.New(cfg)}
}

// Handle routes GET / to health check and POST / to judge.
func (h *Handler) Handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))

	case http.MethodPost:
		h.judge(w, r)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) judge(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Unwrap the outer JSON envelope
	var wrapper messageWrapper
	if err := json.Unmarshal(body, &wrapper); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Parse the proto-text message field into JudgeRequest
	var req pb.JudgeRequest
	if err := prototext.Unmarshal([]byte(wrapper.Message), &req); err != nil {
		http.Error(w, "invalid proto-text: "+err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("judge: lang=%s testcases=%d", req.Language, len(req.Testcases))

	// Execute
	resp := h.exec.Run(&req)

	// Serialize response back to proto-text
	txtBytes, err := prototext.Marshal(resp)
	if err != nil {
		http.Error(w, "proto marshal error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Wrap in JSON envelope
	out, err := json.Marshal(messageWrapper{Message: string(txtBytes)})
	if err != nil {
		http.Error(w, "json marshal error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
}
