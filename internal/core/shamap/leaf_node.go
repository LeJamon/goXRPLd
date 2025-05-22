package shamap

import (
	"encoding/hex"
	"fmt"
	"github.com/LeJamon/goXRPLd/internal/protocol"
)

// -----------------------------------------------------------------------------
// AccountStateLeafNode

type AccountStateLeafNode struct {
	BaseNode
	item *SHAMapItem
}

func (n *AccountStateLeafNode) SerializeForWire() []byte {
	//TODO implement me
	panic("implement me")
}

func (n *AccountStateLeafNode) SerializeWithPrefix() []byte {
	//TODO implement me
	panic("implement me")
}

func NewAccountStateLeafNode(item *SHAMapItem) *AccountStateLeafNode {
	n := &AccountStateLeafNode{item: item}
	n.UpdateHash()
	return n
}

func (n *AccountStateLeafNode) IsLeaf() bool  { return true }
func (n *AccountStateLeafNode) IsInner() bool { return false }

func (n *AccountStateLeafNode) GetItem() *SHAMapItem { return n.item }

func (n *AccountStateLeafNode) SetItem(item *SHAMapItem) bool {
	oldHash := n.hash
	n.item = item
	n.UpdateHash()
	return n.hash != oldHash
}

func (n *AccountStateLeafNode) UpdateHash() {
	key := n.item.Key()
	n.setHash(protocol.HashPrefixLeafNode[:], n.item.Data(), key[:])
}

func (n *AccountStateLeafNode) Type() SHAMapNodeType {
	return tnACCOUNT_STATE
}

func (n *AccountStateLeafNode) Invariants(isRoot bool) error {
	if n.item == nil {
		return fmt.Errorf("account state leaf has nil item")
	}
	return nil
}

func (n *AccountStateLeafNode) String(id NodeID) string {
	key := n.item.Key()
	return fmt.Sprintf("AccountStateLeafNode ID: %s\nHash: %s\nKey: %s\n",
		id.String(), hex.EncodeToString(n.hash[:]), hex.EncodeToString(key[:]))
}

func (n *AccountStateLeafNode) Clone() TreeNode {
	return NewAccountStateLeafNode(n.item.Clone())
}

// -----------------------------------------------------------------------------
// TxLeafNode (transaction without metadata)

type TxLeafNode struct {
	BaseNode
	item *SHAMapItem
}

func (n *TxLeafNode) SerializeForWire() []byte {
	//TODO implement me
	panic("implement me")
}

func (n *TxLeafNode) SerializeWithPrefix() []byte {
	//TODO implement me
	panic("implement me")
}

func NewTxLeafNode(item *SHAMapItem) *TxLeafNode {
	n := &TxLeafNode{item: item}
	n.UpdateHash()
	return n
}

func (n *TxLeafNode) IsLeaf() bool  { return true }
func (n *TxLeafNode) IsInner() bool { return false }

func (n *TxLeafNode) GetItem() *SHAMapItem { return n.item }

func (n *TxLeafNode) SetItem(item *SHAMapItem) bool {
	oldHash := n.hash
	n.item = item
	n.UpdateHash()
	return n.hash != oldHash
}

func (n *TxLeafNode) UpdateHash() {
	n.setHash(protocol.HashPrefixLeafNode[:], n.item.Data())
}

func (n *TxLeafNode) Type() SHAMapNodeType {
	return tnTRANSACTION_NM
}

func (n *TxLeafNode) Invariants(isRoot bool) error {
	if n.item == nil {
		return fmt.Errorf("tx leaf has nil item")
	}
	return nil
}

func (n *TxLeafNode) String(id NodeID) string {
	key := n.item.Key()
	return fmt.Sprintf("TxLeafNode ID: %s\nHash: %s\nKey: %s\n",
		id.String(), hex.EncodeToString(n.hash[:]), hex.EncodeToString(key[:]))
}

func (n *TxLeafNode) Clone() TreeNode {
	return NewTxLeafNode(n.item.Clone())
}

// -----------------------------------------------------------------------------
// TxPlusMetaLeafNode (transaction with metadata)

type TxPlusMetaLeafNode struct {
	BaseNode
	item *SHAMapItem
}

func (n *TxPlusMetaLeafNode) SerializeForWire() []byte {
	//TODO implement me
	panic("implement me")
}

func (n *TxPlusMetaLeafNode) SerializeWithPrefix() []byte {
	//TODO implement me
	panic("implement me")
}

func NewTxPlusMetaLeafNode(item *SHAMapItem) *TxPlusMetaLeafNode {
	n := &TxPlusMetaLeafNode{item: item}
	n.UpdateHash()
	return n
}

func (n *TxPlusMetaLeafNode) IsLeaf() bool  { return true }
func (n *TxPlusMetaLeafNode) IsInner() bool { return false }

func (n *TxPlusMetaLeafNode) GetItem() *SHAMapItem { return n.item }

func (n *TxPlusMetaLeafNode) SetItem(item *SHAMapItem) bool {
	oldHash := n.hash
	n.item = item
	n.UpdateHash()
	return n.hash != oldHash
}

func (n *TxPlusMetaLeafNode) UpdateHash() {
	key := n.item.Key()
	n.setHash(protocol.HashPrefixLeafNode[:], n.item.Data(), key[:])
}

func (n *TxPlusMetaLeafNode) Type() SHAMapNodeType {
	return tnTRANSACTION_MD
}

func (n *TxPlusMetaLeafNode) Invariants(isRoot bool) error {
	if n.item == nil {
		return fmt.Errorf("tx+meta leaf has nil item")
	}
	return nil
}

func (n *TxPlusMetaLeafNode) String(id NodeID) string {
	key := n.item.Key()
	return fmt.Sprintf("TxPlusMetaLeafNode ID: %s\nHash: %s\nKey: %s\n",
		id.String(), hex.EncodeToString(n.hash[:]), hex.EncodeToString(key[:]))
}

func (n *TxPlusMetaLeafNode) Clone() TreeNode {
	return NewTxPlusMetaLeafNode(n.item.Clone())
}

func GetItemFromLeafNode(node TreeNode) *SHAMapItem {
	if node == nil || !node.IsLeaf() {
		return nil
	}
	switch n := node.(type) {
	case *AccountStateLeafNode:
		return n.GetItem()
	case *TxLeafNode:
		return n.GetItem()
	case *TxPlusMetaLeafNode:
		return n.GetItem()
	default:
		return nil
	}
}
