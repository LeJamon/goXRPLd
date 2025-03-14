package jsonrpc

import (
	"encoding/json"
	"net/http"
)

// Server represents a JSON-RPC server.
type Server struct {
	handler *XRPLHandler
}

// NewServer creates a new JSON-RPC server instance.
func NewServer(handler *XRPLHandler) *Server {
	return &Server{handler: handler}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		JsonRPC string      `json:"jsonrpc"`
		Method  string      `json:"method"`
		Params  interface{} `json:"params"`
		ID      interface{} `json:"id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, nil, -32700, "Parse error", nil)
		return
	}

	result, err := s.handler.Handle(req.Method, req.Params)
	if err != nil {
		writeError(w, req.ID, -32603, err.Error(), nil)
		return
	}

	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"result":  result,
		"id":      req.ID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func writeError(w http.ResponseWriter, id interface{}, code int, message string, data interface{}) {
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
			"data":    data,
		},
		"id": id,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
