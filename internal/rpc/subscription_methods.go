package rpc

import (
	"encoding/json"
	"log"
	"regexp"
	"sync"
)

// SubscribeMethod handles the subscribe RPC command (WebSocket only)
type SubscribeMethod struct{}

func (m *SubscribeMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	// This method should only be called through WebSocket context
	// The actual implementation is in the WebSocket handler
	return nil, NewRpcError(RpcNOT_SUPPORTED, "notSupported", "notSupported",
		"subscribe is only available via WebSocket")
}

func (m *SubscribeMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *SubscribeMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// UnsubscribeMethod handles the unsubscribe RPC command (WebSocket only)
type UnsubscribeMethod struct{}

func (m *UnsubscribeMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	// This method should only be called through WebSocket context
	// The actual implementation is in the WebSocket handler
	return nil, NewRpcError(RpcNOT_SUPPORTED, "notSupported", "notSupported",
		"unsubscribe is only available via WebSocket")
}

func (m *UnsubscribeMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *UnsubscribeMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// WebSocket-specific subscription handling

// Connection represents a WebSocket connection with subscription state
type Connection struct {
	ID           string
	Subscriptions map[SubscriptionType]SubscriptionConfig
	SendChannel  chan []byte
	CloseChannel chan struct{}
}

// SubscriptionConfig holds configuration for a specific subscription
type SubscriptionConfig struct {
	Type      SubscriptionType
	Accounts  []string
	Books     []BookRequest
	Streams   []SubscriptionType
	URL       string
	Username  string
	Password  string
}

// SubscribeResponse represents the response to a subscribe command
type SubscribeResponse struct {
	Status      string `json:"status"`
	LedgerIndex uint32 `json:"ledger_index,omitempty"`
	LedgerHash  string `json:"ledger_hash,omitempty"`
	LedgerTime  uint32 `json:"ledger_time,omitempty"`
	FeeBase     uint64 `json:"fee_base,omitempty"`
	ReserveBase uint64 `json:"reserve_base,omitempty"`
	ReserveInc  uint64 `json:"reserve_inc,omitempty"`
}

// SubscriptionManager handles WebSocket subscriptions
type SubscriptionManager struct {
	connections map[string]*Connection
	mu          sync.RWMutex
}

// validXRPLAddressRegex matches classic XRPL addresses (r-addresses)
// Classic addresses are base58-encoded and start with 'r', are 25-35 characters long
var validXRPLAddressRegex = regexp.MustCompile(`^r[1-9A-HJ-NP-Za-km-z]{24,34}$`)

// IsValidXRPLAddress validates an XRPL account address
func IsValidXRPLAddress(address string) bool {
	return validXRPLAddressRegex.MatchString(address)
}

// HandleSubscribe processes WebSocket subscribe commands
func (sm *SubscriptionManager) HandleSubscribe(conn *Connection, request SubscriptionRequest) *RpcError {
	// Validate and process stream subscriptions
	validStreams := map[SubscriptionType]bool{
		SubLedger:       true,
		SubTransactions: true,
		SubValidations:  true,
		SubManifests:    true,
		SubPeerStatus:   true,
		SubConsensus:    true,
	}

	// Parse and validate streams array
	for _, stream := range request.Streams {
		if !validStreams[stream] {
			return RpcErrorInvalidParams("Unknown stream type: " + string(stream))
		}
	}

	// Parse and validate accounts array
	if len(request.Accounts) > 0 {
		for _, account := range request.Accounts {
			if !IsValidXRPLAddress(account) {
				return RpcErrorInvalidParams("Invalid account address: " + account)
			}
		}
	}

	// Parse and validate accounts_proposed array
	if len(request.AccountsProposed) > 0 {
		for _, account := range request.AccountsProposed {
			if !IsValidXRPLAddress(account) {
				return RpcErrorInvalidParams("Invalid account address in accounts_proposed: " + account)
			}
		}
	}

	// Parse and validate books array with taker_pays/taker_gets
	if len(request.Books) > 0 {
		for _, book := range request.Books {
			// Validate taker_pays
			if len(book.TakerPays) == 0 {
				return RpcErrorInvalidParams("Book subscription requires taker_pays")
			}
			var takerPays map[string]interface{}
			if err := json.Unmarshal(book.TakerPays, &takerPays); err != nil {
				return RpcErrorInvalidParams("Invalid taker_pays format: " + err.Error())
			}
			// Validate currency field exists
			if _, ok := takerPays["currency"]; !ok {
				return RpcErrorInvalidParams("taker_pays must specify currency")
			}

			// Validate taker_gets
			if len(book.TakerGets) == 0 {
				return RpcErrorInvalidParams("Book subscription requires taker_gets")
			}
			var takerGets map[string]interface{}
			if err := json.Unmarshal(book.TakerGets, &takerGets); err != nil {
				return RpcErrorInvalidParams("Invalid taker_gets format: " + err.Error())
			}
			// Validate currency field exists
			if _, ok := takerGets["currency"]; !ok {
				return RpcErrorInvalidParams("taker_gets must specify currency")
			}

			// Validate issuer is present for non-XRP currencies
			if currency, ok := takerPays["currency"].(string); ok && currency != "XRP" {
				if _, hasIssuer := takerPays["issuer"]; !hasIssuer {
					return RpcErrorInvalidParams("taker_pays requires issuer for non-XRP currency")
				}
				if issuer, ok := takerPays["issuer"].(string); ok && !IsValidXRPLAddress(issuer) {
					return RpcErrorInvalidParams("Invalid issuer address in taker_pays: " + issuer)
				}
			}
			if currency, ok := takerGets["currency"].(string); ok && currency != "XRP" {
				if _, hasIssuer := takerGets["issuer"]; !hasIssuer {
					return RpcErrorInvalidParams("taker_gets requires issuer for non-XRP currency")
				}
				if issuer, ok := takerGets["issuer"].(string); ok && !IsValidXRPLAddress(issuer) {
					return RpcErrorInvalidParams("Invalid issuer address in taker_gets: " + issuer)
				}
			}
		}
	}

	// Store subscriptions properly in SubscriptionConfig
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Process stream subscriptions
	for _, stream := range request.Streams {
		conn.Subscriptions[stream] = SubscriptionConfig{
			Type: stream,
		}
	}

	// Process account subscriptions
	if len(request.Accounts) > 0 {
		// Merge with existing account subscriptions if any
		existingConfig, exists := conn.Subscriptions[SubAccounts]
		if exists {
			// Append new accounts to existing list (avoiding duplicates)
			accountSet := make(map[string]bool)
			for _, acc := range existingConfig.Accounts {
				accountSet[acc] = true
			}
			for _, acc := range request.Accounts {
				if !accountSet[acc] {
					existingConfig.Accounts = append(existingConfig.Accounts, acc)
				}
			}
			conn.Subscriptions[SubAccounts] = existingConfig
		} else {
			conn.Subscriptions[SubAccounts] = SubscriptionConfig{
				Type:     SubAccounts,
				Accounts: request.Accounts,
			}
		}
	}

	// Process order book subscriptions
	if len(request.Books) > 0 {
		// Merge with existing book subscriptions if any
		existingConfig, exists := conn.Subscriptions[SubOrderBooks]
		if exists {
			existingConfig.Books = append(existingConfig.Books, request.Books...)
			conn.Subscriptions[SubOrderBooks] = existingConfig
		} else {
			conn.Subscriptions[SubOrderBooks] = SubscriptionConfig{
				Type:  SubOrderBooks,
				Books: request.Books,
			}
		}
	}

	// Store URL callback configuration if provided
	if request.URL != "" {
		// URL subscriptions are stored as a special config
		conn.Subscriptions[SubscriptionType("url")] = SubscriptionConfig{
			Type:     SubscriptionType("url"),
			URL:      request.URL,
			Username: request.URLUsername,
			Password: request.URLPassword,
		}
	}

	return nil
}

// GetSubscribeResponse generates a subscription confirmation response with current state
func (sm *SubscriptionManager) GetSubscribeResponse(ledgerIndex uint32, ledgerHash string, ledgerTime uint32, feeBase, reserveBase, reserveInc uint64) *SubscribeResponse {
	return &SubscribeResponse{
		Status:      "success",
		LedgerIndex: ledgerIndex,
		LedgerHash:  ledgerHash,
		LedgerTime:  ledgerTime,
		FeeBase:     feeBase,
		ReserveBase: reserveBase,
		ReserveInc:  reserveInc,
	}
}

// HandleUnsubscribe processes WebSocket unsubscribe commands
func (sm *SubscriptionManager) HandleUnsubscribe(conn *Connection, request SubscriptionRequest) *RpcError {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Remove stream subscriptions
	for _, stream := range request.Streams {
		delete(conn.Subscriptions, stream)
	}

	// Remove account subscriptions if accounts list provided
	if len(request.Accounts) > 0 {
		if config, exists := conn.Subscriptions[SubAccounts]; exists {
			// Remove specific accounts from subscription
			remaining := make([]string, 0)
			removeSet := make(map[string]bool)
			for _, acc := range request.Accounts {
				removeSet[acc] = true
			}
			for _, acc := range config.Accounts {
				if !removeSet[acc] {
					remaining = append(remaining, acc)
				}
			}
			if len(remaining) == 0 {
				delete(conn.Subscriptions, SubAccounts)
			} else {
				config.Accounts = remaining
				conn.Subscriptions[SubAccounts] = config
			}
		}
	}

	// Remove book subscriptions if books list provided
	if len(request.Books) > 0 {
		if config, exists := conn.Subscriptions[SubOrderBooks]; exists {
			// Remove specific books from subscription
			// Compare books by their taker_pays and taker_gets JSON
			remaining := make([]BookRequest, 0)
			for _, existingBook := range config.Books {
				shouldRemove := false
				for _, removeBook := range request.Books {
					if string(existingBook.TakerPays) == string(removeBook.TakerPays) &&
						string(existingBook.TakerGets) == string(removeBook.TakerGets) {
						shouldRemove = true
						break
					}
				}
				if !shouldRemove {
					remaining = append(remaining, existingBook)
				}
			}
			if len(remaining) == 0 {
				delete(conn.Subscriptions, SubOrderBooks)
			} else {
				config.Books = remaining
				conn.Subscriptions[SubOrderBooks] = config
			}
		}
	}

	// Remove URL subscription if URL provided
	if request.URL != "" {
		delete(conn.Subscriptions, SubscriptionType("url"))
	}

	return nil
}

// BroadcastMessage sends a message to all subscribed connections
func (sm *SubscriptionManager) BroadcastMessage(msgType SubscriptionType, message StreamMessage) {
	// Marshal the message once for efficiency
	data, err := json.Marshal(message)
	if err != nil {
		return
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for connID, conn := range sm.connections {
		shouldSend := false

		switch msgType {
		case SubLedger:
			// For ledger stream: send to all SubLedger subscribers
			if _, subscribed := conn.Subscriptions[SubLedger]; subscribed {
				shouldSend = true
			}

		case SubTransactions:
			// For transactions stream: send to all SubTransactions subscribers
			if _, subscribed := conn.Subscriptions[SubTransactions]; subscribed {
				shouldSend = true
			}

		case SubAccounts:
			// For account events: filter by subscribed accounts
			if config, subscribed := conn.Subscriptions[SubAccounts]; subscribed {
				// Check if the message's account is in the subscribed accounts list
				if message.Account != "" {
					for _, subscribedAccount := range config.Accounts {
						if subscribedAccount == message.Account {
							shouldSend = true
							break
						}
					}
				}
			}

		case SubOrderBooks:
			// For book changes: send to all SubOrderBooks subscribers
			// Further filtering by specific books can be done based on message content
			if _, subscribed := conn.Subscriptions[SubOrderBooks]; subscribed {
				shouldSend = true
			}

		case SubValidations:
			// For validations stream: send to all SubValidations subscribers
			if _, subscribed := conn.Subscriptions[SubValidations]; subscribed {
				shouldSend = true
			}

		case SubManifests:
			// For manifests stream: send to all SubManifests subscribers
			if _, subscribed := conn.Subscriptions[SubManifests]; subscribed {
				shouldSend = true
			}

		case SubPeerStatus:
			// For peer_status stream: send to all SubPeerStatus subscribers
			if _, subscribed := conn.Subscriptions[SubPeerStatus]; subscribed {
				shouldSend = true
			}

		case SubConsensus:
			// For consensus stream: send to all SubConsensus subscribers
			if _, subscribed := conn.Subscriptions[SubConsensus]; subscribed {
				shouldSend = true
			}

		default:
			// Unknown message type, check if directly subscribed
			if _, subscribed := conn.Subscriptions[msgType]; subscribed {
				shouldSend = true
			}
		}

		if shouldSend {
			select {
			case conn.SendChannel <- data:
				// Message sent successfully
			case <-conn.CloseChannel:
				// Connection is closed, will be cleaned up by RemoveConnection
				_ = connID // Acknowledge connID usage
			default:
				// Channel is full, connection may be slow - skip this message
			}
		}
	}
}

// BroadcastToAccount sends a message to all connections subscribed to a specific account
func (sm *SubscriptionManager) BroadcastToAccount(account string, message interface{}) {
	data, err := json.Marshal(message)
	if err != nil {
		return
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, conn := range sm.connections {
		if config, subscribed := conn.Subscriptions[SubAccounts]; subscribed {
			for _, subscribedAccount := range config.Accounts {
				if subscribedAccount == account {
					select {
					case conn.SendChannel <- data:
						// Message sent
					case <-conn.CloseChannel:
						// Connection closed
					default:
						// Channel full
					}
					break
				}
			}
		}
	}
}

// GetSubscribersForStream returns all connections subscribed to a stream
func (sm *SubscriptionManager) GetSubscribersForStream(stream string) []*WebSocketConnection {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var subscribers []*WebSocketConnection
	streamType := SubscriptionType(stream)

	for _, conn := range sm.connections {
		if _, subscribed := conn.Subscriptions[streamType]; subscribed {
			// Create a WebSocketConnection wrapper for the legacy Connection
			wsConn := &WebSocketConnection{
				ID:          conn.ID,
				sendChannel: conn.SendChannel,
			}
			subscribers = append(subscribers, wsConn)
		}
	}

	return subscribers
}

// GetSubscribersForAccount returns connections subscribed to an account
func (sm *SubscriptionManager) GetSubscribersForAccount(account string) []*WebSocketConnection {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var subscribers []*WebSocketConnection

	for _, conn := range sm.connections {
		if config, subscribed := conn.Subscriptions[SubAccounts]; subscribed {
			for _, subscribedAccount := range config.Accounts {
				if subscribedAccount == account {
					wsConn := &WebSocketConnection{
						ID:          conn.ID,
						sendChannel: conn.SendChannel,
					}
					subscribers = append(subscribers, wsConn)
					break
				}
			}
		}
	}

	return subscribers
}

// IsSubscribed checks if a connection is subscribed to a stream
func (sm *SubscriptionManager) IsSubscribed(connID string, stream string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	conn, exists := sm.connections[connID]
	if !exists {
		return false
	}

	_, subscribed := conn.Subscriptions[SubscriptionType(stream)]
	return subscribed
}

// GetConnectionSubscriptions returns all subscriptions for a connection
func (sm *SubscriptionManager) GetConnectionSubscriptions(connID string) map[SubscriptionType]SubscriptionConfig {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	conn, exists := sm.connections[connID]
	if !exists {
		return nil
	}

	// Return a copy to avoid race conditions
	result := make(map[SubscriptionType]SubscriptionConfig)
	for k, v := range conn.Subscriptions {
		result[k] = v
	}
	return result
}

// AddConnection adds a new WebSocket connection
func (sm *SubscriptionManager) AddConnection(conn *Connection) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.connections[conn.ID] = conn
}

// RemoveConnection removes a WebSocket connection
func (sm *SubscriptionManager) RemoveConnection(connID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.removeConnectionLocked(connID)
}

// removeConnectionLocked removes a connection without acquiring the lock
// Must be called with sm.mu held
func (sm *SubscriptionManager) removeConnectionLocked(connID string) {
	if conn, exists := sm.connections[connID]; exists {
		// Close the close channel to signal the connection is being removed
		select {
		case <-conn.CloseChannel:
			// Already closed
		default:
			close(conn.CloseChannel)
		}
		delete(sm.connections, connID)
	}
}

// GetConnection returns a connection by ID
func (sm *SubscriptionManager) GetConnection(connID string) *Connection {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.connections[connID]
}

// ConnectionCount returns the number of active connections
func (sm *SubscriptionManager) ConnectionCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.connections)
}

// BroadcastToStream sends pre-marshaled data to all subscribers of a specific stream
// The filter function is optional and can be used for additional filtering
func (sm *SubscriptionManager) BroadcastToStream(streamType SubscriptionType, data []byte, filter func(*Connection) bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, conn := range sm.connections {
		// Check if subscribed to this stream
		if _, subscribed := conn.Subscriptions[streamType]; !subscribed {
			continue
		}

		// Apply optional filter
		if filter != nil && !filter(conn) {
			continue
		}

		// Send the message
		select {
		case conn.SendChannel <- data:
			// Message sent successfully
		case <-conn.CloseChannel:
			// Connection is closed
		default:
			// Channel is full, skip this message
			log.Printf("Skipping slow connection %s for stream %s", conn.ID, streamType)
		}
	}
}

// BroadcastToAccounts sends pre-marshaled data to subscribers of specific accounts
func (sm *SubscriptionManager) BroadcastToAccounts(data []byte, accounts []string) {
	if len(accounts) == 0 {
		return
	}

	// Create a set for O(1) lookup
	accountSet := make(map[string]bool)
	for _, acc := range accounts {
		accountSet[acc] = true
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, conn := range sm.connections {
		config, subscribed := conn.Subscriptions[SubAccounts]
		if !subscribed {
			continue
		}

		// Check if any of the connection's subscribed accounts match
		shouldSend := false
		for _, subscribedAccount := range config.Accounts {
			if accountSet[subscribedAccount] {
				shouldSend = true
				break
			}
		}

		if !shouldSend {
			continue
		}

		// Send the message
		select {
		case conn.SendChannel <- data:
			// Message sent
		case <-conn.CloseChannel:
			// Connection closed
		default:
			// Channel full
			log.Printf("Skipping slow connection %s for account subscription", conn.ID)
		}
	}
}

// BroadcastToAccountsProposed sends pre-marshaled data to subscribers of accounts_proposed
func (sm *SubscriptionManager) BroadcastToAccountsProposed(data []byte, accounts []string) {
	if len(accounts) == 0 {
		return
	}

	// Create a set for O(1) lookup
	accountSet := make(map[string]bool)
	for _, acc := range accounts {
		accountSet[acc] = true
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, conn := range sm.connections {
		// Check accounts_proposed subscriptions
		// These are stored in a special way - we need to check AccountsProposed field
		config, subscribed := conn.Subscriptions[SubscriptionType("accounts_proposed")]
		if !subscribed {
			continue
		}

		// Check if any of the subscribed accounts match
		shouldSend := false
		for _, subscribedAccount := range config.Accounts {
			if accountSet[subscribedAccount] {
				shouldSend = true
				break
			}
		}

		if !shouldSend {
			continue
		}

		// Send the message
		select {
		case conn.SendChannel <- data:
			// Message sent
		case <-conn.CloseChannel:
			// Connection closed
		default:
			// Channel full
			log.Printf("Skipping slow connection %s for accounts_proposed subscription", conn.ID)
		}
	}
}

// BroadcastToOrderBook sends pre-marshaled data to subscribers of a specific order book
func (sm *SubscriptionManager) BroadcastToOrderBook(data []byte, takerGets, takerPays CurrencySpec) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, conn := range sm.connections {
		config, subscribed := conn.Subscriptions[SubOrderBooks]
		if !subscribed {
			continue
		}

		// Check if any of the subscribed books match
		shouldSend := false
		for _, book := range config.Books {
			if bookMatchesCurrency(book, takerGets, takerPays) {
				shouldSend = true
				break
			}
		}

		if !shouldSend {
			continue
		}

		// Send the message
		select {
		case conn.SendChannel <- data:
			// Message sent
		case <-conn.CloseChannel:
			// Connection closed
		default:
			// Channel full
			log.Printf("Skipping slow connection %s for order book subscription", conn.ID)
		}
	}
}

// CurrencySpec specifies a currency for order book matching
type CurrencySpec struct {
	Currency string
	Issuer   string // Empty for XRP
}

// bookMatchesCurrency checks if a book subscription matches the given currency pair
func bookMatchesCurrency(book BookRequest, takerGets, takerPays CurrencySpec) bool {
	// Parse taker_gets from the book subscription
	var getsMap map[string]interface{}
	if err := json.Unmarshal(book.TakerGets, &getsMap); err != nil {
		return false
	}

	// Parse taker_pays from the book subscription
	var paysMap map[string]interface{}
	if err := json.Unmarshal(book.TakerPays, &paysMap); err != nil {
		return false
	}

	// Check if currencies match
	getsCurrency, _ := getsMap["currency"].(string)
	paysCurrency, _ := paysMap["currency"].(string)
	getsIssuer, _ := getsMap["issuer"].(string)
	paysIssuer, _ := paysMap["issuer"].(string)

	// Compare currencies
	if getsCurrency != takerGets.Currency || paysCurrency != takerPays.Currency {
		return false
	}

	// For non-XRP currencies, also compare issuers
	if getsCurrency != "XRP" && getsIssuer != takerGets.Issuer {
		return false
	}
	if paysCurrency != "XRP" && paysIssuer != takerPays.Issuer {
		return false
	}

	return true
}

// GetSubscriberCount returns the number of subscribers for a specific stream type
func (sm *SubscriptionManager) GetSubscriberCount(streamType SubscriptionType) int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	count := 0
	for _, conn := range sm.connections {
		if _, subscribed := conn.Subscriptions[streamType]; subscribed {
			count++
		}
	}
	return count
}

// GetAllStreamSubscriberCounts returns counts for all stream types
func (sm *SubscriptionManager) GetAllStreamSubscriberCounts() map[SubscriptionType]int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	counts := make(map[SubscriptionType]int)
	for _, conn := range sm.connections {
		for streamType := range conn.Subscriptions {
			counts[streamType]++
		}
	}
	return counts
}

// Note: Event types for subscription system are defined in events.go
// Types include: LedgerCloseEvent, TransactionEvent, ValidationEvent,
// ManifestEvent, PeerStatusEvent, ConsensusEvent, PathFindEvent