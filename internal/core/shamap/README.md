# SHAMap Specification

## Overview

SHAMap is a specialized data structure used in the XRP Ledger that combines features of:

- **Merkle Tree**: Each non-leaf node is labeled with the hash of its children.
- **Patricia Trie (Radix Tree)**: Efficient prefix tree with path compression.
- **Hexary Tree**: Each inner node can have up to 16 children (one for each hex nibble).

SHAMaps support fast, verifiable access to state and transaction data in a ledger.

---

## Data Structure

### Node Types

#### 1. `InnerNode`

- Represents a branch point in the tree.
- Contains up to 16 children (0–15), corresponding to hex nibbles.
- Empty slots represent absent paths.
- Stores:
  - A bitmap or mask indicating which children are present.
  - The hash of each present child.
- Hash is computed from the ordered set of child hashes.

#### 2. `LeafNode`

- Contains a single key-value pair (a `SHAMapItem`).
- Represents the endpoint of a path in the tree.
- Hash is computed from both the key and value.

---

### `SHAMapItem`

Encapsulates the actual ledger data stored at a leaf node.

- `key`: 256-bit hash used to place the item in the trie.
- `value`: Serialized ledger object (e.g., Account, Offer, NFT).

---

### Tree Structure

- Keys are 256-bit hashes → 64 hex nibbles → max depth = 64.
- Each level corresponds to one hex digit (4 bits).
- Nodes compress paths where possible (if only one child exists).
- Leaf nodes appear conceptually at depth 64.

---

## Core Operations

### Item-Level

- `HasItem(hash) → bool`: Check if an item exists.
- `GetItem(hash) → SHAMapItem`: Retrieve the item.
- `AddItem(item)`: Insert a new item.
- `UpdateItem(item)`: Replace an existing item.
- `RemoveItem(hash)`: Remove an item.

### Tree-Level

- `GetHash() → Hash256`: Return root hash.
- `Snapshot(mutable: bool) → SHAMap`: Clone the tree for concurrent reads/writes.
- `SetImmutable()`: Prevent further modifications.

### Sync & Proof

- `GetMissingNodes(max: int) → List<NodeID>`: Identify missing nodes.
- `AddKnownNode(nodeID, data)`: Add a known node from a peer.
- `PathProof(hash) → Proof[]`: Generate a Merkle proof for a key.
- `VerifyProof(hash, proof) → bool`: Verify the proof.

---

## Technical Requirements

### Hashing

- `InnerNode` hash = hash of ordered child hashes.
- `LeafNode` hash = hash of key + value.
- Hashes must be deterministic and collision-resistant.

### Serialization

- Nodes must serialize to canonical binary format.
- Types and flags must be encoded to distinguish node types.
- Compatible with XRPL wire protocol.

### Copy-on-Write Semantics

- Tree snapshots share nodes until a mutation occurs.
- On write, affected nodes are cloned (copy-on-write).
- Enables ledger versioning and efficient memory usage.

### Thread Safety

- Concurrent reads: Safe without locks.
- Writes: Must be synchronized.
- Reader-writer patterns or atomic reference counters may be used.

---

## Performance Guidelines

### Memory

- Nodes may be reference-counted or garbage-collected.
- Implement LRU or generational caching.
- Avoid deep copies — prefer pointer or shared structures.

### Computation

- Cache node hashes to avoid recomputation.
- Use path compression to minimize tree depth.
- Optimize tree traversal with nibble indexing.

### Storage

- Support lazy loading via persistent `NodeStore`.
- Mark dirty nodes for background flush.
- Use efficient encoding for disk I/O.

---

## Notes

- SHAMap is deterministic: same inputs always produce the same root hash.
- Used for both ledger state (`stateMap`) and transaction history (`txMap`).
- Essential for consensus and ledger verification.
