package jsonrpc

import (
	"fmt"
	"github.com/LeJamon/goXRPLd/internal/server/methods"
)

// XRPLHandler handles XRPL-related JSON-RPC methods.
type XRPLHandler struct {
	methods map[string]func(interface{}) (interface{}, error)
}

// NewXRPLHandler initializes a new XRPLHandler instance.
func NewXRPLHandler() *XRPLHandler {
	h := &XRPLHandler{
		methods: make(map[string]func(interface{}) (interface{}, error)),
	}

	// Register available methods.
	h.methods["account_info"] = methods.HandleAccountInfo

	return h
}

// Handle dispatches a JSON-RPC method to the appropriate handler.
func (h *XRPLHandler) Handle(method string, params interface{}) (interface{}, error) {
	handler, exists := h.methods[method]
	if !exists {
		return nil, fmt.Errorf("method %s not found", method)
	}
	return handler(params)
}

func (h *XRPLHandler) handleSubmit(params interface{}) (interface{}, error) {
	// Mocked submit method for demonstration.
	response := map[string]interface{}{
		"status": "success",
		"txHash": "1234ABCD...",
	}
	return response, nil
}
