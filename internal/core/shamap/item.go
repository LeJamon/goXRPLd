package shamap

import (
	"errors"
	"sync/atomic"
)

// SHAMapItem represents a leaf-level item stored in the SHAMap.
type SHAMapItem struct {
	key      [32]byte // The unique identifier (e.g., hash) for this item
	blob     []byte   // The raw data payload
	size     uint32   // Cached size of the blob
	refCount atomic.Uint32
}

// NewSHAMapItem constructs a new SHAMapItem.
func NewSHAMapItem(key [32]byte, data []byte) *SHAMapItem {
	item := &SHAMapItem{
		key:      key,
		blob:     make([]byte, len(data)),
		size:     uint32(len(data)),
		refCount: atomic.Uint32{},
	}
	copy(item.blob, data)
	item.refCount.Store(1)
	return item
}

// Key returns the key of the item.
func (s *SHAMapItem) Key() [32]byte {
	return s.key
}

// Data returns the raw data stored in the item.
func (s *SHAMapItem) Data() []byte {
	return s.blob
}

// Size returns the size of the data blob.
func (s *SHAMapItem) Size() int {
	return int(s.size)
}

// AddRef increases the reference count.
func (s *SHAMapItem) AddRef() {
	s.refCount.Add(1)
}

// Release decreases the reference count and returns whether it reached zero.
func (s *SHAMapItem) Release() (shouldFree bool, err error) {
	prev := s.refCount.Load()
	if prev == 0 {
		return false, errors.New("invalid refcount: already zero")
	}
	if s.refCount.Add(^uint32(0)) == 0 {
		return true, nil
	}
	return false, nil
}

// Clone creates a deep copy of the item.
func (s *SHAMapItem) Clone() *SHAMapItem {
	return NewSHAMapItem(s.key, s.blob)
}
