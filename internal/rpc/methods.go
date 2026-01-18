package rpc

// registerAllMethods registers all XRPL RPC methods
// This function is called by NewServer to set up the complete method registry
func (s *Server) registerAllMethods() {
	// Server Information Methods
	s.registry.Register("server_info", &ServerInfoMethod{})
	s.registry.Register("server_state", &ServerStateMethod{})
	s.registry.Register("ping", &PingMethod{})
	s.registry.Register("random", &RandomMethod{})
	s.registry.Register("server_definitions", &ServerDefinitionsMethod{})
	s.registry.Register("feature", &FeatureMethod{})
	s.registry.Register("fee", &FeeMethod{})
	
	// Ledger Methods
	s.registry.Register("ledger", &LedgerMethod{})
	s.registry.Register("ledger_closed", &LedgerClosedMethod{})
	s.registry.Register("ledger_current", &LedgerCurrentMethod{})
	s.registry.Register("ledger_data", &LedgerDataMethod{})
	s.registry.Register("ledger_entry", &LedgerEntryMethod{})
	s.registry.Register("ledger_range", &LedgerRangeMethod{})
	
	// Account Methods
	s.registry.Register("account_info", &AccountInfoMethod{})
	s.registry.Register("account_channels", &AccountChannelsMethod{})
	s.registry.Register("account_currencies", &AccountCurrenciesMethod{})
	s.registry.Register("account_lines", &AccountLinesMethod{})
	s.registry.Register("account_nfts", &AccountNftsMethod{})
	s.registry.Register("account_objects", &AccountObjectsMethod{})
	s.registry.Register("account_offers", &AccountOffersMethod{})
	s.registry.Register("account_tx", &AccountTxMethod{})
	s.registry.Register("gateway_balances", &GatewayBalancesMethod{})
	s.registry.Register("noripple_check", &NoRippleCheckMethod{})
	
	// Transaction Methods
	s.registry.Register("tx", &TxMethod{})
	s.registry.Register("tx_history", &TxHistoryMethod{})
	s.registry.Register("submit", &SubmitMethod{})
	s.registry.Register("submit_multisigned", &SubmitMultisignedMethod{})
	s.registry.Register("sign", &SignMethod{})
	s.registry.Register("sign_for", &SignForMethod{})
	s.registry.Register("transaction_entry", &TransactionEntryMethod{})
	
	// Path and Order Book Methods
	s.registry.Register("book_offers", &BookOffersMethod{})
	s.registry.Register("path_find", &PathFindMethod{})
	s.registry.Register("ripple_path_find", &RipplePathFindMethod{})
	
	// Channel Methods
	s.registry.Register("channel_authorize", &ChannelAuthorizeMethod{})
	s.registry.Register("channel_verify", &ChannelVerifyMethod{})
	
	// Subscription Methods (WebSocket only)
	s.registry.Register("subscribe", &SubscribeMethod{})
	s.registry.Register("unsubscribe", &UnsubscribeMethod{})
	
	// Utility Methods
	s.registry.Register("wallet_propose", &WalletProposeMethod{})
	s.registry.Register("deposit_authorized", &DepositAuthorizedMethod{})
	s.registry.Register("nft_buy_offers", &NftBuyOffersMethod{})
	s.registry.Register("nft_sell_offers", &NftSellOffersMethod{})
	s.registry.Register("nft_history", &NftHistoryMethod{})
	s.registry.Register("nfts_by_issuer", &NftsByIssuerMethod{})
	
	// Generic Methods
	s.registry.Register("json", &JsonMethod{})
	
	// Standalone mode methods
	s.registry.Register("ledger_accept", &LedgerAcceptMethod{})

	// Admin Methods (require admin role)
	s.registry.Register("stop", &StopMethod{})
	s.registry.Register("validation_create", &ValidationCreateMethod{})
	s.registry.Register("manifest", &ManifestMethod{})
	s.registry.Register("peer_reservations_add", &PeerReservationsAddMethod{})
	s.registry.Register("peer_reservations_del", &PeerReservationsDelMethod{})
	s.registry.Register("peer_reservations_list", &PeerReservationsListMethod{})
	s.registry.Register("peers", &PeersMethod{})
	s.registry.Register("consensus_info", &ConsensusInfoMethod{})
	s.registry.Register("validator_list_sites", &ValidatorListSitesMethod{})
	s.registry.Register("validators", &ValidatorsMethod{})
	
	// Reporting Mode Methods
	s.registry.Register("download_shard", &DownloadShardMethod{})
	s.registry.Register("crawl_shards", &CrawlShardsMethod{})
	
	// Clio-specific Methods (if running in clio mode)
	s.registry.Register("nft_info", &NftInfoMethod{})
	s.registry.Register("ledger_index", &LedgerIndexMethod{})

	// =========================================================================
	// Additional Methods (added for rippled compatibility)
	// =========================================================================

	// Server/Network Methods
	s.registry.Register("fetch_info", &FetchInfoMethod{})
	s.registry.Register("connect", &ConnectMethod{})
	s.registry.Register("print", &PrintMethod{})

	// Ledger Methods
	s.registry.Register("ledger_header", &LedgerHeaderMethod{})
	s.registry.Register("ledger_request", &LedgerRequestMethod{})
	s.registry.Register("ledger_cleaner", &LedgerCleanerMethod{})
	s.registry.Register("ledger_diff", &LedgerDiffMethod{})

	// Account Methods
	s.registry.Register("owner_info", &OwnerInfoMethod{})

	// Transaction Methods
	s.registry.Register("simulate", &SimulateMethod{})
	s.registry.Register("tx_reduce_relay", &TxReduceRelayMethod{})

	// Validator Methods
	s.registry.Register("validator_info", &ValidatorInfoMethod{})
	s.registry.Register("unl_list", &UnlListMethod{})

	// Admin/Operational Methods
	s.registry.Register("can_delete", &CanDeleteMethod{})
	s.registry.Register("get_counts", &GetCountsMethod{})
	s.registry.Register("log_level", &LogLevelMethod{})
	s.registry.Register("log_rotate", &LogRotateMethod{})
	s.registry.Register("black_list", &BlackListMethod{})

	// Feature-specific Methods (depend on unimplemented ledger entry types)
	s.registry.Register("amm_info", &AMMInfoMethod{})
	s.registry.Register("vault_info", &VaultInfoMethod{})
	s.registry.Register("get_aggregate_price", &GetAggregatePriceMethod{})
}