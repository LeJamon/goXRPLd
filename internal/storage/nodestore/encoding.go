package nodestore

import (
	"encoding/binary"
	"fmt"
)

// nodeEncodingHeaderSize is the number of bytes in the encoding header.
// Format: [nodeType:1][ledgerSeq:4] = 5 bytes
const nodeEncodingHeaderSize = 5

// encodeNodeData serializes a node for storage in the kvstore.
// Format: [nodeType:1][ledgerSeq:4][data:N]
func encodeNodeData(n *Node) []byte {
	buf := make([]byte, nodeEncodingHeaderSize+len(n.Data))
	buf[0] = byte(n.Type)
	binary.BigEndian.PutUint32(buf[1:5], n.LedgerSeq)
	copy(buf[nodeEncodingHeaderSize:], n.Data)
	return buf
}

// decodeNodeData deserializes a node from kvstore data.
func decodeNodeData(hash Hash256, data []byte) (*Node, error) {
	if len(data) < nodeEncodingHeaderSize {
		return nil, fmt.Errorf("%w: data too short (%d bytes)", ErrDataCorrupt, len(data))
	}
	nodeType := NodeType(data[0])
	ledgerSeq := binary.BigEndian.Uint32(data[1:5])
	nodeData := make([]byte, len(data)-nodeEncodingHeaderSize)
	copy(nodeData, data[nodeEncodingHeaderSize:])
	return &Node{
		Type:      nodeType,
		Hash:      hash,
		Data:      nodeData,
		LedgerSeq: ledgerSeq,
	}, nil
}
