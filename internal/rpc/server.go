package rpc

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Server handles HTTP JSON-RPC 2.0 requests
type Server struct {
	registry *MethodRegistry
	timeout  time.Duration
}

// NewServer creates a new RPC server with the given timeout
func NewServer(timeout time.Duration) *Server {
	server := &Server{
		registry: NewMethodRegistry(),
		timeout:  timeout,
	}
	
	// Register all RPC methods
	server.registerAllMethods()
	
	return server
}

// ServeHTTP implements http.Handler interface
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers to match rippled
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")
	
	// Handle preflight requests
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	
	// Only accept POST and GET methods
	if r.Method != "POST" && r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	// Handle GET request (for simple queries like server_info)
	if r.Method == "GET" {
		s.handleGetRequest(w, r)
		return
	}
	
	// Handle POST request (standard JSON-RPC)
	s.handlePostRequest(w, r)
}

// handleGetRequest processes GET requests with query parameters
func (s *Server) handleGetRequest(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	method := query.Get("command")
	
	if method == "" {
		// Default to server_info for GET requests without command
		method = "server_info"
	}
	
	// Create RPC context
	ctx := &RpcContext{
		Context:    r.Context(),
		Role:       RoleGuest, // TODO: Implement proper role detection
		ApiVersion: DefaultApiVersion,
		IsAdmin:    false, // TODO: Implement admin detection
		ClientIP:   getClientIP(r),
	}
	
	// Execute method
	result, rpcErr := s.executeMethod(method, nil, ctx)
	
	// Send response
	response := JsonRpcResponse{
		JsonRpc: "2.0",
		ID:      1,
	}
	
	if rpcErr != nil {
		response.Error = rpcErr
	} else {
		response.Result = result
	}
	
	s.writeResponse(w, response)
}

// handlePostRequest processes POST requests with JSON-RPC payload
func (s *Server) handlePostRequest(w http.ResponseWriter, r *http.Request) {
	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeError(w, RpcErrorInternal("Failed to read request body"), nil)
		return
	}
	defer r.Body.Close()
	
	// Parse JSON-RPC request
	var request JsonRpcRequest
	if err := json.Unmarshal(body, &request); err != nil {
		s.writeError(w, NewRpcError(RpcPARSE_ERROR, "parseError", "parseError", "Invalid JSON"), nil)
		return
	}
	
	// Validate JSON-RPC version
	if request.JsonRpc != "2.0" {
		s.writeError(w, RpcErrorInvalidParams("Invalid jsonrpc version"), request.ID)
		return
	}
	
	// Create RPC context
	ctx := &RpcContext{
		Context:    r.Context(),
		Role:       RoleGuest, // TODO: Implement proper role detection
		ApiVersion: DefaultApiVersion,
		IsAdmin:    false, // TODO: Implement admin detection
		ClientIP:   getClientIP(r),
	}
	
	// Parse API version from params if present
	if request.Params != nil {
		var params map[string]interface{}
		if err := json.Unmarshal(request.Params, &params); err == nil {
			if apiVer, ok := params["api_version"]; ok {
				if ver, ok := apiVer.(float64); ok {
					ctx.ApiVersion = int(ver)
				}
			}
		}
	}
	
	// Execute method
	result, rpcErr := s.executeMethod(request.Method, request.Params, ctx)
	
	// Send response
	response := JsonRpcResponse{
		JsonRpc: "2.0",
		ID:      request.ID,
	}
	
	if rpcErr != nil {
		response.Error = rpcErr
	} else {
		response.Result = result
	}
	
	s.writeResponse(w, response)
}

// executeMethod executes an RPC method with the given parameters
func (s *Server) executeMethod(method string, params json.RawMessage, ctx *RpcContext) (interface{}, *RpcError) {
	// Get method handler
	handler, exists := s.registry.Get(method)
	if !exists {
		return nil, RpcErrorMethodNotFound(method)
	}
	
	// Check role permissions
	if ctx.Role < handler.RequiredRole() {
		return nil, NewRpcError(RpcCOMMAND_UNTRUSTED, "commandUntrusted", "commandUntrusted", 
			fmt.Sprintf("Method '%s' requires higher privileges", method))
	}
	
	// Check API version support
	supportedVersions := handler.SupportedApiVersions()
	if len(supportedVersions) > 0 {
		supported := false
		for _, version := range supportedVersions {
			if ctx.ApiVersion == version {
				supported = true
				break
			}
		}
		if !supported {
			return nil, RpcErrorInvalidApiVersion(strconv.Itoa(ctx.ApiVersion))
		}
	}
	
	// Execute handler
	return handler.Handle(ctx, params)
}

// writeResponse writes a JSON-RPC response
func (s *Server) writeResponse(w http.ResponseWriter, response JsonRpcResponse) {
	responseData, err := json.Marshal(response)
	if err != nil {
		log.Printf("Failed to marshal response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	
	w.WriteHeader(http.StatusOK)
	w.Write(responseData)
}

// writeError writes an error response
func (s *Server) writeError(w http.ResponseWriter, rpcErr *RpcError, id interface{}) {
	response := JsonRpcResponse{
		JsonRpc: "2.0",
		Error:   rpcErr,
		ID:      id,
	}
	s.writeResponse(w, response)
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		return strings.TrimSpace(ips[0])
	}
	
	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	
	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	
	return ip
}