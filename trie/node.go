package trie

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/relab/hotstuff"
	"golang.org/x/crypto/sha3"
)

// NodeType represents the type of trie node
type NodeType int

const (
	// BranchNode has up to 16 children + optional value
	BranchNode NodeType = iota
	// ExtensionNode compresses a path with common prefix
	ExtensionNode
	// LeafNode stores a key-value pair
	LeafNode
	// EmptyNode represents no data
	EmptyNode
)

// Node represents a node in the Merkle Patricia Trie
type Node interface {
	Type() NodeType
	Hash() hotstuff.Hash
	Encode() []byte
	String() string
}

// BranchNodeStruct represents a branch node with up to 16 children
type BranchNodeStruct struct {
	Children [16]Node      // 16 possible nibble values (0-F)
	Value    []byte        // Optional value stored at this node
	hash     hotstuff.Hash // Cached hash
	dirty    bool          // Whether hash needs recalculation
}

// ExtensionNodeStruct represents an extension node that compresses paths
type ExtensionNodeStruct struct {
	Key   []byte        // Shared key prefix (nibbles)
	Child Node          // Single child node
	hash  hotstuff.Hash // Cached hash
	dirty bool          // Whether hash needs recalculation
}

// LeafNodeStruct represents a leaf node with key-value data
type LeafNodeStruct struct {
	Key   []byte        // Remaining key path (nibbles)
	Value []byte        // Stored value
	hash  hotstuff.Hash // Cached hash
	dirty bool          // Whether hash needs recalculation
}

// EmptyNodeStruct represents an empty node
type EmptyNodeStruct struct{}

// NewBranchNode creates a new branch node
func NewBranchNode() *BranchNodeStruct {
	return &BranchNodeStruct{
		dirty: true,
	}
}

// NewExtensionNode creates a new extension node
func NewExtensionNode(key []byte, child Node) *ExtensionNodeStruct {
	return &ExtensionNodeStruct{
		Key:   key,
		Child: child,
		dirty: true,
	}
}

// NewLeafNode creates a new leaf node
func NewLeafNode(key []byte, value []byte) *LeafNodeStruct {
	return &LeafNodeStruct{
		Key:   key,
		Value: value,
		dirty: true,
	}
}

// EmptyNodeInstance singleton instance
var EmptyNodeInstance = &EmptyNodeStruct{}

// BranchNode implementations

func (n *BranchNodeStruct) Type() NodeType {
	return BranchNode
}

func (n *BranchNodeStruct) Hash() hotstuff.Hash {
	if n.dirty || n.hash == (hotstuff.Hash{}) {
		n.hash = n.calculateHash()
		n.dirty = false
	}
	return n.hash
}

func (n *BranchNodeStruct) calculateHash() hotstuff.Hash {
	hasher := sha3.NewLegacyKeccak256()
	encoded := n.Encode()
	hasher.Write(encoded)
	
	var hash hotstuff.Hash
	copy(hash[:], hasher.Sum(nil))
	return hash
}

func (n *BranchNodeStruct) Encode() []byte {
	var buf bytes.Buffer
	
	// Encode type
	buf.WriteByte(byte(BranchNode))
	
	// Encode children hashes
	for _, child := range n.Children {
		if child == nil || child.Type() == EmptyNode {
			buf.Write(make([]byte, 32)) // Empty hash
		} else {
			hash := child.Hash()
			buf.Write(hash[:])
		}
	}
	
	// Encode value length and value
	if len(n.Value) > 0 {
		buf.WriteByte(byte(len(n.Value)))
		buf.Write(n.Value)
	} else {
		buf.WriteByte(0)
	}
	
	return buf.Bytes()
}

func (n *BranchNodeStruct) String() string {
	childCount := 0
	for _, child := range n.Children {
		if child != nil && child.Type() != EmptyNode {
			childCount++
		}
	}
	
	valueStr := ""
	if len(n.Value) > 0 {
		valueStr = fmt.Sprintf(", value=%d bytes", len(n.Value))
	}
	
	return fmt.Sprintf("Branch(children=%d%s)", childCount, valueStr)
}

func (n *BranchNodeStruct) SetChild(index int, child Node) {
	if index >= 0 && index < 16 {
		n.Children[index] = child
		n.dirty = true
	}
}

func (n *BranchNodeStruct) GetChild(index int) Node {
	if index >= 0 && index < 16 {
		child := n.Children[index]
	if child == nil {
		return EmptyNodeInstance
	}
		return child
	}
	return EmptyNodeInstance
}

func (n *BranchNodeStruct) SetValue(value []byte) {
	n.Value = make([]byte, len(value))
	copy(n.Value, value)
	n.dirty = true
}

// ExtensionNode implementations

func (n *ExtensionNodeStruct) Type() NodeType {
	return ExtensionNode
}

func (n *ExtensionNodeStruct) Hash() hotstuff.Hash {
	if n.dirty || n.hash == (hotstuff.Hash{}) {
		n.hash = n.calculateHash()
		n.dirty = false
	}
	return n.hash
}

func (n *ExtensionNodeStruct) calculateHash() hotstuff.Hash {
	hasher := sha3.NewLegacyKeccak256()
	encoded := n.Encode()
	hasher.Write(encoded)
	
	var hash hotstuff.Hash
	copy(hash[:], hasher.Sum(nil))
	return hash
}

func (n *ExtensionNodeStruct) Encode() []byte {
	var buf bytes.Buffer
	
	// Encode type
	buf.WriteByte(byte(ExtensionNode))
	
	// Encode key length and key
	buf.WriteByte(byte(len(n.Key)))
	buf.Write(n.Key)
	
	// Encode child hash
	if n.Child != nil && n.Child.Type() != EmptyNode {
		hash := n.Child.Hash()
		buf.Write(hash[:])
	} else {
		buf.Write(make([]byte, 32)) // Empty hash
	}
	
	return buf.Bytes()
}

func (n *ExtensionNodeStruct) String() string {
	return fmt.Sprintf("Extension(key=%s, child=%s)", 
		hex.EncodeToString(n.Key), n.Child.String())
}

// LeafNode implementations

func (n *LeafNodeStruct) Type() NodeType {
	return LeafNode
}

func (n *LeafNodeStruct) Hash() hotstuff.Hash {
	if n.dirty || n.hash == (hotstuff.Hash{}) {
		n.hash = n.calculateHash()
		n.dirty = false
	}
	return n.hash
}

func (n *LeafNodeStruct) calculateHash() hotstuff.Hash {
	hasher := sha3.NewLegacyKeccak256()
	encoded := n.Encode()
	hasher.Write(encoded)
	
	var hash hotstuff.Hash
	copy(hash[:], hasher.Sum(nil))
	return hash
}

func (n *LeafNodeStruct) Encode() []byte {
	var buf bytes.Buffer
	
	// Encode type
	buf.WriteByte(byte(LeafNode))
	
	// Encode key length and key
	buf.WriteByte(byte(len(n.Key)))
	buf.Write(n.Key)
	
	// Encode value length and value
	valueLen := len(n.Value)
	if valueLen > 255 {
		// For large values, use 2 bytes for length
		buf.WriteByte(255)
		buf.WriteByte(byte(valueLen >> 8))
		buf.WriteByte(byte(valueLen & 0xFF))
	} else {
		buf.WriteByte(byte(valueLen))
	}
	buf.Write(n.Value)
	
	return buf.Bytes()
}

func (n *LeafNodeStruct) String() string {
	return fmt.Sprintf("Leaf(key=%s, value=%d bytes)", 
		hex.EncodeToString(n.Key), len(n.Value))
}

// EmptyNode implementations

func (n *EmptyNodeStruct) Type() NodeType {
	return EmptyNode
}

func (n *EmptyNodeStruct) Hash() hotstuff.Hash {
	return hotstuff.Hash{} // Empty hash
}

func (n *EmptyNodeStruct) Encode() []byte {
	return []byte{byte(EmptyNode)}
}

func (n *EmptyNodeStruct) String() string {
	return "Empty"
}

// Utility functions

// keyToNibbles converts a byte key to nibbles (4-bit values)
func keyToNibbles(key []byte) []byte {
	nibbles := make([]byte, len(key)*2)
	for i, b := range key {
		nibbles[i*2] = b >> 4     // High nibble
		nibbles[i*2+1] = b & 0x0F // Low nibble
	}
	return nibbles
}

// nibblesFromHex converts a hex string to nibbles
func nibblesFromHex(hexStr string) ([]byte, error) {
	if len(hexStr)%2 != 0 {
		hexStr = "0" + hexStr // Pad with leading zero
	}
	
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, err
	}
	
	return keyToNibbles(bytes), nil
}

// nibblesEqual compares two nibble slices
func nibblesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// commonPrefix finds the longest common prefix between two nibble slices
func commonPrefix(a, b []byte) int {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	
	for i := 0; i < minLen; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return minLen
}

// isNibbleValid checks if a byte is a valid nibble (0-15)
func isNibbleValid(nibble byte) bool {
	return nibble <= 15
}
