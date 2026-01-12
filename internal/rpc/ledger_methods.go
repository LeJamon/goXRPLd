package rpc

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
)

// LedgerMethod handles the ledger RPC method
type LedgerMethod struct{}

func (m *LedgerMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	// Parse parameters
	var request struct {
		LedgerSpecifier
		Accounts     bool `json:"accounts,omitempty"`
		Full         bool `json:"full,omitempty"`
		Transactions bool `json:"transactions,omitempty"`
		Expand       bool `json:"expand,omitempty"`
		OwnerFunds   bool `json:"owner_funds,omitempty"`
		Binary       bool `json:"binary,omitempty"`
		Queue        bool `json:"queue,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	// Check if ledger service is available
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// Determine which ledger to retrieve
	var targetLedger LedgerReader
	var validated bool
	var err error

	if request.LedgerHash != "" {
		// Look up by hash
		hashBytes, decErr := hex.DecodeString(request.LedgerHash)
		if decErr != nil || len(hashBytes) != 32 {
			return nil, RpcErrorInvalidParams("Invalid ledger_hash")
		}
		var hash [32]byte
		copy(hash[:], hashBytes)
		targetLedger, err = Services.Ledger.GetLedgerByHash(hash)
		if err != nil {
			return nil, &RpcError{Code: -1, ErrorString: "lgrNotFound", Message: "Ledger not found"}
		}
		validated = targetLedger.IsValidated()
	} else {
		// Look up by index
		ledgerIndex := request.LedgerIndex.String()
		if ledgerIndex == "" {
			ledgerIndex = "validated"
		}

		switch ledgerIndex {
		case "validated":
			seq := Services.Ledger.GetValidatedLedgerIndex()
			if seq == 0 {
				return nil, &RpcError{Code: -1, ErrorString: "lgrNotFound", Message: "No validated ledger"}
			}
			targetLedger, err = Services.Ledger.GetLedgerBySequence(seq)
			validated = true
		case "current":
			seq := Services.Ledger.GetCurrentLedgerIndex()
			targetLedger, err = Services.Ledger.GetLedgerBySequence(seq)
			validated = false
		case "closed":
			seq := Services.Ledger.GetClosedLedgerIndex()
			targetLedger, err = Services.Ledger.GetLedgerBySequence(seq)
			validated = targetLedger != nil && targetLedger.IsValidated()
		default:
			// Parse as number
			seq, parseErr := strconv.ParseUint(ledgerIndex, 10, 32)
			if parseErr != nil {
				return nil, RpcErrorInvalidParams("Invalid ledger_index")
			}
			targetLedger, err = Services.Ledger.GetLedgerBySequence(uint32(seq))
			if err != nil {
				return nil, &RpcError{Code: -1, ErrorString: "lgrNotFound", Message: "Ledger not found"}
			}
			validated = targetLedger.IsValidated()
		}
	}

	if err != nil || targetLedger == nil {
		return nil, &RpcError{Code: -1, ErrorString: "lgrNotFound", Message: "Ledger not found"}
	}

	// Build ledger info
	hash := targetLedger.Hash()
	parent := targetLedger.ParentHash()
	ledgerHash := hex.EncodeToString(hash[:])
	parentHash := hex.EncodeToString(parent[:])

	ledgerInfo := map[string]interface{}{
		"accepted":     true,
		"close_flags":  0,
		"closed":       targetLedger.IsClosed(),
		"hash":         ledgerHash,
		"ledger_hash":  ledgerHash,
		"ledger_index": strconv.FormatUint(uint64(targetLedger.Sequence()), 10),
		"parent_hash":  parentHash,
		"seqNum":       strconv.FormatUint(uint64(targetLedger.Sequence()), 10),
		"totalCoins":   strconv.FormatUint(targetLedger.TotalDrops(), 10),
		"total_coins":  strconv.FormatUint(targetLedger.TotalDrops(), 10),
	}

	response := map[string]interface{}{
		"ledger":       ledgerInfo,
		"ledger_hash":  ledgerHash,
		"ledger_index": targetLedger.Sequence(),
		"validated":    validated,
	}

	// Add transactions if requested (placeholder - would need transaction iteration)
	if request.Transactions {
		response["ledger"].(map[string]interface{})["transactions"] = []interface{}{}
	}

	// Add queue if requested and this is current ledger
	if request.Queue {
		response["queue_data"] = []interface{}{}
	}

	return response, nil
}

func (m *LedgerMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *LedgerMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// LedgerClosedMethod handles the ledger_closed RPC method
type LedgerClosedMethod struct{}

func (m *LedgerClosedMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	// Check if ledger service is available
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// Get the closed ledger index
	seq := Services.Ledger.GetClosedLedgerIndex()
	if seq == 0 {
		return nil, &RpcError{Code: -1, ErrorString: "lgrNotFound", Message: "No closed ledger"}
	}

	// Get the ledger to retrieve its hash
	ledger, err := Services.Ledger.GetLedgerBySequence(seq)
	if err != nil {
		return nil, &RpcError{Code: -1, ErrorString: "lgrNotFound", Message: "Closed ledger not found"}
	}

	hash := ledger.Hash()
	response := map[string]interface{}{
		"ledger_hash":  hex.EncodeToString(hash[:]),
		"ledger_index": seq,
	}

	return response, nil
}

func (m *LedgerClosedMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *LedgerClosedMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// LedgerCurrentMethod handles the ledger_current RPC method
type LedgerCurrentMethod struct{}

func (m *LedgerCurrentMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	// Check if ledger service is available
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// Get the current (open) ledger index
	seq := Services.Ledger.GetCurrentLedgerIndex()
	if seq == 0 {
		return nil, &RpcError{Code: -1, ErrorString: "lgrNotFound", Message: "No current ledger"}
	}

	response := map[string]interface{}{
		"ledger_current_index": seq,
	}

	return response, nil
}

func (m *LedgerCurrentMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *LedgerCurrentMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// LedgerDataMethod handles the ledger_data RPC method
type LedgerDataMethod struct{}

func (m *LedgerDataMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	// Parse parameters
	var request struct {
		LedgerSpecifier
		Binary bool        `json:"binary,omitempty"`
		Limit  uint32      `json:"limit,omitempty"`
		Marker interface{} `json:"marker,omitempty"`
		Type   string      `json:"type,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	// Check if ledger service is available
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// Validate limit
	if request.Limit > 2048 {
		request.Limit = 2048
	}
	if request.Limit == 0 {
		request.Limit = 256 // Default limit
	}

	// Determine ledger index to use
	ledgerIndex := "current"
	if request.LedgerIndex != "" {
		ledgerIndex = request.LedgerIndex.String()
	}

	// Parse marker as string
	markerStr := ""
	if request.Marker != nil {
		if m, ok := request.Marker.(string); ok {
			markerStr = m
		}
	}

	// Get ledger data from the ledger service
	result, err := Services.Ledger.GetLedgerData(ledgerIndex, request.Limit, markerStr)
	if err != nil {
		return nil, RpcErrorInternal("Failed to get ledger data: " + err.Error())
	}

	// Build state array based on binary flag
	state := make([]map[string]interface{}, len(result.State))
	for i, item := range result.State {
		if request.Binary {
			// Binary format: just data and index as hex
			state[i] = map[string]interface{}{
				"data":  hex.EncodeToString(item.Data),
				"index": item.Index,
			}
		} else {
			// JSON format: deserialize the ledger entry
			jsonObj, err := deserializeLedgerEntry(item.Data)
			if err != nil {
				println(err.Error())
				// Fallback to binary format if deserialization fails
				state[i] = map[string]interface{}{
					"data":  hex.EncodeToString(item.Data),
					"index": item.Index,
				}
			} else {
				// Add index field to the deserialized object
				if objMap, ok := jsonObj.(map[string]interface{}); ok {
					objMap["index"] = item.Index
					state[i] = objMap
				} else {
					state[i] = map[string]interface{}{
						"data":  hex.EncodeToString(item.Data),
						"index": item.Index,
					}
				}
			}
		}
	}

	response := map[string]interface{}{
		"ledger_hash":  hex.EncodeToString(result.LedgerHash[:]),
		"ledger_index": result.LedgerIndex,
		"state":        state,
		"validated":    result.Validated,
	}

	// Include ledger header info on first query (when no marker was provided)
	if result.LedgerHeader != nil {
		if request.Binary {
			// Binary format: include ledger_data as hex serialization
			response["ledger"] = map[string]interface{}{
				"ledger_data": formatLedgerHeaderBinary(result.LedgerHeader),
				"closed":      result.LedgerHeader.Closed,
			}
		} else {
			// JSON format: include full ledger header fields
			response["ledger"] = map[string]interface{}{
				"account_hash":          hex.EncodeToString(result.LedgerHeader.AccountHash[:]),
				"close_flags":           result.LedgerHeader.CloseFlags,
				"close_time":            result.LedgerHeader.CloseTime,
				"close_time_human":      result.LedgerHeader.CloseTimeHuman,
				"close_time_iso":        result.LedgerHeader.CloseTimeISO,
				"close_time_resolution": result.LedgerHeader.CloseTimeResolution,
				"closed":                result.LedgerHeader.Closed,
				"ledger_hash":           hex.EncodeToString(result.LedgerHeader.LedgerHash[:]),
				"ledger_index":          result.LedgerHeader.LedgerIndex,
				"parent_close_time":     result.LedgerHeader.ParentCloseTime,
				"parent_hash":           hex.EncodeToString(result.LedgerHeader.ParentHash[:]),
				"total_coins":           fmt.Sprintf("%d", result.LedgerHeader.TotalCoins),
				"transaction_hash":      hex.EncodeToString(result.LedgerHeader.TransactionHash[:]),
			}
		}
	}

	if result.Marker != "" {
		response["marker"] = result.Marker
	}

	return response, nil
}

// formatLedgerHeaderBinary creates a hex-encoded binary representation of ledger header
func formatLedgerHeaderBinary(hdr *LedgerHeaderInfo) string {
	// This is a simplified binary format - real implementation would match rippled's serialization
	var buf []byte

	// Sequence (4 bytes)
	seqBytes := make([]byte, 4)
	seqBytes[0] = byte(hdr.LedgerIndex >> 24)
	seqBytes[1] = byte(hdr.LedgerIndex >> 16)
	seqBytes[2] = byte(hdr.LedgerIndex >> 8)
	seqBytes[3] = byte(hdr.LedgerIndex)
	buf = append(buf, seqBytes...)

	// Total coins (8 bytes)
	coinsBytes := make([]byte, 8)
	coinsBytes[0] = byte(hdr.TotalCoins >> 56)
	coinsBytes[1] = byte(hdr.TotalCoins >> 48)
	coinsBytes[2] = byte(hdr.TotalCoins >> 40)
	coinsBytes[3] = byte(hdr.TotalCoins >> 32)
	coinsBytes[4] = byte(hdr.TotalCoins >> 24)
	coinsBytes[5] = byte(hdr.TotalCoins >> 16)
	coinsBytes[6] = byte(hdr.TotalCoins >> 8)
	coinsBytes[7] = byte(hdr.TotalCoins)
	buf = append(buf, coinsBytes...)

	// Parent hash, tx hash, account hash
	buf = append(buf, hdr.ParentHash[:]...)
	buf = append(buf, hdr.TransactionHash[:]...)
	buf = append(buf, hdr.AccountHash[:]...)

	// Parent close time (4 bytes)
	pctBytes := make([]byte, 4)
	pct := uint32(hdr.ParentCloseTime)
	pctBytes[0] = byte(pct >> 24)
	pctBytes[1] = byte(pct >> 16)
	pctBytes[2] = byte(pct >> 8)
	pctBytes[3] = byte(pct)
	buf = append(buf, pctBytes...)

	// Close time (4 bytes)
	ctBytes := make([]byte, 4)
	ct := uint32(hdr.CloseTime)
	ctBytes[0] = byte(ct >> 24)
	ctBytes[1] = byte(ct >> 16)
	ctBytes[2] = byte(ct >> 8)
	ctBytes[3] = byte(ct)
	buf = append(buf, ctBytes...)

	// Close time resolution (1 byte) and close flags (1 byte)
	buf = append(buf, byte(hdr.CloseTimeResolution))
	buf = append(buf, hdr.CloseFlags)

	return hex.EncodeToString(buf)
}

// deserializeLedgerEntry converts binary ledger entry data to JSON format
func deserializeLedgerEntry(data []byte) (interface{}, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}

	// Use the binary codec's Decode function to convert binary to JSON
	println("HEX VALUE TO DECODE: ", hex.EncodeToString(data))
	return binarycodec.Decode(hex.EncodeToString(data))
}

func (m *LedgerDataMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *LedgerDataMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// LedgerEntryMethod handles the ledger_entry RPC method
type LedgerEntryMethod struct{}

func (m *LedgerEntryMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	// Parse parameters - this method supports multiple ways to specify objects
	var request struct {
		LedgerSpecifier
		// Object specification methods (mutually exclusive):
		Index          string `json:"index,omitempty"`        // Direct object ID
		AccountRoot    string `json:"account_root,omitempty"` // Account address
		Check          string `json:"check,omitempty"`        // Check object ID
		DepositPreauth struct {
			Owner      string `json:"owner"`
			Authorized string `json:"authorized"`
		} `json:"deposit_preauth,omitempty"`
		DirectoryNode string `json:"directory,omitempty"` // Directory ID
		Escrow        struct {
			Owner string `json:"owner"`
			Seq   uint32 `json:"seq"`
		} `json:"escrow,omitempty"`
		Offer struct {
			Account string `json:"account"`
			Seq     uint32 `json:"seq"`
		} `json:"offer,omitempty"`
		PaymentChannel string `json:"payment_channel,omitempty"` // Channel ID
		RippleState    struct {
			Accounts []string `json:"accounts"`
			Currency string   `json:"currency"`
		} `json:"ripple_state,omitempty"`
		SignerList string `json:"signer_list,omitempty"` // Account address
		Ticket     struct {
			Account  string `json:"account"`
			TicketID uint32 `json:"ticket_id"`
		} `json:"ticket,omitempty"`
		NFTPage string `json:"nft_page,omitempty"` // NFT page ID

		Binary bool `json:"binary,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	// Check if ledger service is available
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// Determine ledger index to use
	ledgerIndex := "validated"
	if request.LedgerIndex != "" {
		ledgerIndex = request.LedgerIndex.String()
	}

	// Determine the entry key
	var entryKey [32]byte
	var keySet bool

	// Check for direct index specification
	if request.Index != "" {
		decoded, err := hex.DecodeString(request.Index)
		if err != nil || len(decoded) != 32 {
			return nil, RpcErrorInvalidParams("Invalid index: must be 64-character hex string")
		}
		copy(entryKey[:], decoded)
		keySet = true
	}

	// Check for other object types
	if !keySet && request.Check != "" {
		decoded, err := hex.DecodeString(request.Check)
		if err != nil || len(decoded) != 32 {
			return nil, RpcErrorInvalidParams("Invalid check: must be 64-character hex string")
		}
		copy(entryKey[:], decoded)
		keySet = true
	}

	if !keySet && request.PaymentChannel != "" {
		decoded, err := hex.DecodeString(request.PaymentChannel)
		if err != nil || len(decoded) != 32 {
			return nil, RpcErrorInvalidParams("Invalid payment_channel: must be 64-character hex string")
		}
		copy(entryKey[:], decoded)
		keySet = true
	}

	if !keySet && request.DirectoryNode != "" {
		decoded, err := hex.DecodeString(request.DirectoryNode)
		if err != nil || len(decoded) != 32 {
			return nil, RpcErrorInvalidParams("Invalid directory: must be 64-character hex string")
		}
		copy(entryKey[:], decoded)
		keySet = true
	}

	if !keySet && request.NFTPage != "" {
		decoded, err := hex.DecodeString(request.NFTPage)
		if err != nil || len(decoded) != 32 {
			return nil, RpcErrorInvalidParams("Invalid nft_page: must be 64-character hex string")
		}
		copy(entryKey[:], decoded)
		keySet = true
	}

	if !keySet {
		return nil, RpcErrorInvalidParams("Must specify object by index, check, payment_channel, directory, or nft_page")
	}

	// Get ledger entry from the ledger service
	result, err := Services.Ledger.GetLedgerEntry(entryKey, ledgerIndex)
	if err != nil {
		if err.Error() == "entry not found" {
			return nil, &RpcError{
				Code:    21, // entryNotFound
				Message: "Requested ledger entry not found.",
			}
		}
		return nil, RpcErrorInternal("Failed to get ledger entry: " + err.Error())
	}

	response := map[string]interface{}{
		"index":        result.Index,
		"ledger_hash":  hex.EncodeToString(result.LedgerHash[:]),
		"ledger_index": result.LedgerIndex,
		"validated":    result.Validated,
	}

	if request.Binary {
		response["node_binary"] = result.NodeBinary
	} else {
		response["node"] = hex.EncodeToString(result.Node)
	}

	return response, nil
}

func (m *LedgerEntryMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *LedgerEntryMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// LedgerRangeMethod handles the ledger_range RPC method
type LedgerRangeMethod struct{}

func (m *LedgerRangeMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	// Parse parameters
	var request struct {
		StartLedger uint32 `json:"start_ledger"`
		StopLedger  uint32 `json:"stop_ledger"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	// Validate range
	if request.StartLedger == 0 || request.StopLedger == 0 {
		return nil, RpcErrorInvalidParams("start_ledger and stop_ledger are required")
	}

	if request.StartLedger > request.StopLedger {
		return nil, RpcErrorInvalidParams("start_ledger cannot be greater than stop_ledger")
	}

	// Limit range size to prevent abuse
	if request.StopLedger-request.StartLedger > 1000 {
		return nil, RpcErrorInvalidParams("Ledger range too large (max 1000 ledgers)")
	}

	// Check if ledger service is available
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// Get ledger range from the ledger service
	result, err := Services.Ledger.GetLedgerRange(request.StartLedger, request.StopLedger)
	if err != nil {
		return nil, RpcErrorInternal("Failed to get ledger range: " + err.Error())
	}

	// Build ledgers array
	ledgers := make([]map[string]interface{}, 0, len(result.Hashes))
	for seq, hash := range result.Hashes {
		ledgers = append(ledgers, map[string]interface{}{
			"ledger_index": seq,
			"ledger_hash":  hex.EncodeToString(hash[:]),
		})
	}

	response := map[string]interface{}{
		"ledger_first": result.LedgerFirst,
		"ledger_last":  result.LedgerLast,
		"ledgers":      ledgers,
	}

	return response, nil
}

func (m *LedgerRangeMethod) RequiredRole() Role {
	return RoleAdmin // This method requires admin privileges
}

func (m *LedgerRangeMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}
