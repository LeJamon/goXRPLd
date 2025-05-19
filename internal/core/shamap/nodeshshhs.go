/*package shamap

import (
	"errors"
	"sync"
)

type NodeType uint

const (
	hotUKNOWN           NodeType = 0
	hotLEDGER           NodeType = 1
	hotACCOUNT_NODE     NodeType = 2
	hotTRANSACTION_NODE NodeType = 4
	hotDUMMY            NodeType = 512
)

type NodeObject struct {
	Hash [32]byte
	Blob []byte
	Type NodeType
}

type DecodedBlob struct {
	Key        []byte
	ObjectType NodeType
	ObjectData []byte
}

type EncodedBlob struct {
	Key     []byte
	Payload []byte
	size    uint32
	ptr     *uint8
}

func DecodeBlob(key, value []byte) (*DecodedBlob, error) {
	if len(value) == 0 {
		return nil, errors.New("empty value")
	}

	// Example decoding logic — you'd tailor this to XRPL's actual encoding.
	objType, objData, err := parseNodeObject(value)
	if err != nil {
		return nil, err
	}

	return &DecodedBlob{
		Key:        key,
		ObjectType: objType,
		ObjectData: objData,
	}, nil
}

func (d *DecodedBlob) ToNodeObject() *NodeObject {
	return &NodeObject{
		Hash: decodeHash(d.Key),
		Blob: d.ObjectData,
		Type: uint(d.ObjectType),
	}
}

// TreeNode is the interface for all nodes.
type TreeNode interface {
	IsLeaf() bool
	GetHash() [32]byte
	SetHash(h [32]byte)
}

// InnerNode represents an internal node with children.
type InnerNode struct {
	mutex    sync.RWMutex
	children [BranchFactor]TreeNode
	hash     [32]byte
	dirty    bool
}

func (n *InnerNode) IsLeaf() bool {
	return false
}

func (n *InnerNode) GetHash() [32]byte {
	n.mutex.RLock()
	defer n.mutex.RUnlock()
	return n.hash
}

func (n *InnerNode) SetHash(h [32]byte) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.hash = h
	n.dirty = false
}

// LeafNode represents a leaf node holding an item.
type LeafNode struct {
	hash [32]byte
	item *Item
}

func (n *LeafNode) IsLeaf() bool {
	return true
}

func (n *LeafNode) GetHash() [32]byte {
	return n.hash
}

func (n *LeafNode) SetHash(h [32]byte) {
	n.hash = h
}

// Item represents the leaf payload — you’ll want to adapt this.
type Item struct {
	Key  [32]byte
	Data []byte
}*/
