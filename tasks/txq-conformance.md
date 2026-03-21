# TxQ Conformance Session

## Status
- Started: 22 failing TxQ conformance tests
- Current: 18 failing (4 fixed: straightfoward_positive_case, replace_last_tx, replace_middle_tx, last_ledger_sequence)
- 20 passing (up from 16)

## Fixes Applied

### 1. maxSize initialization (txq/txq.go)
Changed `maxSize` from `uint32` to `*uint32`. Starts as nil (no limit) matching rippled's `std::optional<size_t> maxSize_` which starts as `nullopt`. Queue has no capacity limit until first `processClosedLedger()` call.

### 2. tryClearAccountQueue (txq/apply.go)
Implemented rippled's `tryClearAccountQueueUpThruTx` (TxQ.cpp:518-614). When a high-fee transaction is submitted for an account with queued transactions, it checks whether the total fee paid across all queued + new transactions can cover the escalated series fee. If so, it applies all queued transactions and the new one atomically. This fixes the `terQUEUED` vs `tesSUCCESS` pattern for queue-clearing transactions.

### 3. Time-leap close support (testing/env_submission.go, conformance/runner.go)
Added `CloseWithTimeLeap()` method that calls `ProcessClosedLedger(ctx, true)` instead of `false`. Time-leap closes reset TxQ fee metrics (`txnsExpected`) back toward the minimum, matching rippled's `env.close(env.now() + 5s, 10000ms)`. Added `txqTimeLeapLookup` map to identify which fixture steps need time-leap closes.

### 4. Actual fee level tracking (testing/env_submission.go, testing/env.go)
Added `closingFeeLevels` to track actual fee levels of applied transactions instead of assuming all transactions pay BaseLevel (256). This correctly computes the escalation multiplier (median fee) in `ProcessClosedLedger`, making fee escalation match rippled when high-fee transactions are in the ledger.

### 5. initFee config support (conformance/runner.go, testing/env.go)
Added `txqInitFeeLookup` map and `SetBaseFee`/`SetReserves` methods. Fixtures that use rippled's `initFee()` pattern (255 close steps followed by fee vote) need post-initFee reserves applied. The runner detects the initFee pattern and applies the correct reserves (e.g., base=10, reserve=200, increment=50 drops).

## Remaining Failing Tests (18)

### TxQMetaInfo (9 fail)
1. Sequence_in_queue_and_open_ledger — direct open ledger modify (bypasses TxQ)
2. Ticket_in_queue_and_open_ledger — similar direct modify pattern
3. Zero_reference_fee — zero baseFee edge case
4. clear_queue_failure_(load) — depends on replace_middle_tx chain
5. expiration_replacement — expiration-based gap handling
6. full_queue_gap_handling — gap filling logic
7. scaling — metric computation during scaling
8. Queue_full_drop_penalty — drop penalty during queue management
9. Re-execute_preflight — preflight re-execution during queue processing

### TxQPosNegFlows (9 fail)
1. zero_transaction_fee — fee=0 transactions + blocker logic
2. unexpected_balance_change — balance check differences
3. multi_tx_per_account — balance/blocker edge cases
4. queue_sequence — balance tracking after queue operations
5. blockers_sequence — blocker detection logic differences
6. blockers_ticket — blocker detection with tickets
7. In-flight_balance_checks — in-flight balance computation
8. queue_ticket — ticket queue ordering
9. zero_transaction_fee — fee=0 edge case

## Remaining Root Causes

### Direct open ledger modify (2 tests)
Some rippled tests use `env.app().openLedger().modify()` to apply transactions directly to the open ledger, bypassing the TxQ entirely. The fixture recorder captures these as normal tx steps with tesSUCCESS. The goXRPL runner routes them through TxQ, causing terQUEUED instead of tesSUCCESS.

### Blocker detection differences (3 tests)
The blocker check in goXRPL counts ALL transactions in the account queue, while rippled only counts transactions with seqProxy >= account's current sequence. This causes false-positive blocker detection.

### Fee=0 edge cases (2 tests)
Transactions with fee=0 have fee level 0, which is below BaseLevel (256). During queue drain, these can't meet the base fee requirement. But in rippled, certain mechanisms (localTxs, direct apply) handle these differently.

### Balance tracking after queue operations (4 tests)
The in-flight balance check and balance tracking after queue clear operations has subtle differences from rippled.

### localTxs retry mechanism (several tests)
Rippled retries dropped queue transactions via localTxs. The goXRPL runner's held-txn mechanism doesn't perfectly replicate this.

## Key Files

### goXRPL TxQ
- `internal/txq/txq.go` — main TxQ implementation (maxSize now *uint32)
- `internal/txq/apply.go` — Apply logic + tryClearAccountQueue
- `internal/txq/fee.go` — fee escalation
- `internal/txq/config.go` — configuration
- `internal/txq/process.go` — ProcessClosedLedger + SetMaxSize
- `internal/testing/env_submission.go` — TxQ integration, fee level tracking, time-leap close
- `internal/testing/env.go` — closingFeeLevels field, SetBaseFee, SetReserves

### rippled TxQ (reference)
- `rippled/src/xrpld/app/misc/detail/TxQ.cpp` — main implementation
- `rippled/src/xrpld/app/misc/TxQ.h` — header
- `rippled/src/test/app/TxQ_test.cpp` — tests (shows initFee, time-leap usage)

### Conformance runner
- `internal/testing/conformance/runner.go` — txqMinTxnLookup, txqTimeLeapLookup, txqInitFeeLookup, execClose with time-leap

## Time-Leap Close Mapping
Derived from `rippled/src/test/app/TxQ_test.cpp`:
- queue sequence: step 27
- last ledger sequence: steps 2, 5, 8
- zero transaction fee: steps 2, 4
- scaling: steps 150, 151, 152, 153, 203
- multi tx per account, In-flight balance checks, unexpected balance change, Zero reference fee: step 257 (initFee)
