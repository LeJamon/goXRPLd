package interfaces

type XRPLResponse struct {
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
