package jsonrpc

// XRPLRPCRequest represents a JSON-RPC request
type XRPLRPCRequest struct {
	Method string        `json:"method"`
	Params []interface{} `json:"params,omitempty"`
}

// XRPLRPCResponse represents a JSON-RPC response
type XRPLRPCResponse struct {
	Result     interface{} `json:"result"`
	ID         interface{} `json:"id"`
	APIVersion int         `json:"api_version"`
	Type       string      `json:"type"`
	Warnings   []Warning   `json:"warnings,omitempty"`
}

type Warning struct {
	ID      int    `json:"id"`
	Message string `json:"message"`
}

// RPCError represents a JSON-RPC error
type XRPLRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}
