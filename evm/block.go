package evm

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/txpool"
	"golang.org/x/crypto/sha3"
)

// EVMBlock represents an Ethereum-compatible block structure
type EVMBlock struct {
	// HotStuff consensus fields
	hash     hotstuff.Hash
	parent   hotstuff.Hash
	proposer hotstuff.ID
	cert     hotstuff.QuorumCert
	view     hotstuff.View
	ts       time.Time

	// Ethereum block header fields
	Header EVMBlockHeader `json:"header"`

	// Block body
	Transactions []*txpool.Transaction `json:"transactions"`
	Receipts     []*TransactionReceipt `json:"receipts,omitempty"`
}

// EVMBlockHeader contains Ethereum-compatible block header fields
type EVMBlockHeader struct {
	// Core Ethereum fields
	Number       *big.Int       `json:"number"`        // Block number
	StateRoot    hotstuff.Hash  `json:"stateRoot"`     // Root hash of the state trie
	TxRoot       hotstuff.Hash  `json:"transactionsRoot"` // Root hash of transactions trie
	ReceiptRoot  hotstuff.Hash  `json:"receiptsRoot"`  // Root hash of receipts trie
	LogsBloom    []byte         `json:"logsBloom"`     // Bloom filter for logs
	GasLimit     uint64         `json:"gasLimit"`      // Maximum gas allowed in this block
	GasUsed      uint64         `json:"gasUsed"`       // Total gas used by all transactions
	Timestamp    uint64         `json:"timestamp"`     // Unix timestamp
	ExtraData    []byte         `json:"extraData"`     // Arbitrary data
	BaseFee      *big.Int       `json:"baseFeePerGas"` // EIP-1559 base fee
	
	// Coinbase/beneficiary (proposer address in our case)
	Coinbase txpool.Address `json:"miner"`

	// Difficulty fields (simplified for HotStuff)
	Difficulty   *big.Int `json:"difficulty"`
	TotalDifficulty *big.Int `json:"totalDifficulty,omitempty"`
	
	// Nonce for PoW compatibility (unused but included for RPC compatibility)
	Nonce uint64 `json:"nonce"`

	// Size field for RPC responses
	Size uint64 `json:"size,omitempty"`
}

// TransactionReceipt represents the result of transaction execution
type TransactionReceipt struct {
	TxHash          txpool.Hash     `json:"transactionHash"`
	TxIndex         uint64          `json:"transactionIndex"`
	BlockHash       hotstuff.Hash   `json:"blockHash"`
	BlockNumber     *big.Int        `json:"blockNumber"`
	From            txpool.Address  `json:"from"`
	To              *txpool.Address `json:"to"`
	CumulativeGasUsed uint64        `json:"cumulativeGasUsed"`
	GasUsed         uint64          `json:"gasUsed"`
	ContractAddress *txpool.Address `json:"contractAddress,omitempty"`
	Logs            []*Log          `json:"logs"`
	LogsBloom       []byte          `json:"logsBloom"`
	Status          uint64          `json:"status"` // 1 for success, 0 for failure
	EffectiveGasPrice *big.Int      `json:"effectiveGasPrice"`
}

// Log represents a contract log entry
type Log struct {
	Address     txpool.Address `json:"address"`
	Topics      []hotstuff.Hash `json:"topics"`
	Data        []byte         `json:"data"`
	BlockNumber *big.Int       `json:"blockNumber"`
	TxHash      txpool.Hash    `json:"transactionHash"`
	TxIndex     uint64         `json:"transactionIndex"`
	BlockHash   hotstuff.Hash  `json:"blockHash"`
	LogIndex    uint64         `json:"logIndex"`
	Removed     bool           `json:"removed"`
}

// NewEVMBlock creates a new Ethereum-compatible block
func NewEVMBlock(parent hotstuff.Hash, cert hotstuff.QuorumCert, 
	transactions []*txpool.Transaction, view hotstuff.View, proposer hotstuff.ID,
	stateRoot hotstuff.Hash, gasLimit uint64) *EVMBlock {
	
	// Convert proposer ID to address (simplified mapping)
	var coinbase txpool.Address
	proposerStr := fmt.Sprintf("proposer_%d", proposer)
	if len(proposerStr) > 20 {
		proposerStr = proposerStr[:20]
	}
	copy(coinbase[:], proposerStr)

	// Calculate transaction and receipt roots (simplified)
	txRoot := calculateTransactionRoot(transactions)
	receiptRoot := hotstuff.Hash{} // Will be calculated after execution
	
	// Calculate gas used and logs bloom (will be updated after execution)
	var gasUsed uint64
	for _, tx := range transactions {
		gasUsed += tx.GasLimit // Simplified: assume all gas is used
	}
	
	header := EVMBlockHeader{
		Number:       big.NewInt(int64(view)), // Use view as block number for now
		StateRoot:    stateRoot,
		TxRoot:       txRoot,
		ReceiptRoot:  receiptRoot,
		LogsBloom:    make([]byte, 256), // Empty bloom filter initially
		GasLimit:     gasLimit,
		GasUsed:      gasUsed,
		Timestamp:    uint64(time.Now().Unix()),
		ExtraData:    []byte("HotStuff-EVM"),
		BaseFee:      big.NewInt(1000000000), // 1 gwei base fee
		Coinbase:     coinbase,
		Difficulty:   big.NewInt(1), // Fixed difficulty for HotStuff
		TotalDifficulty: big.NewInt(int64(view)), // Cumulative view count
		Nonce:        0, // No PoW nonce needed
	}

	block := &EVMBlock{
		parent:       parent,
		cert:         cert,
		view:         view,
		proposer:     proposer,
		ts:           time.Now(),
		Header:       header,
		Transactions: transactions,
		Receipts:     make([]*TransactionReceipt, 0), // Will be populated after execution
	}

	// Calculate and cache the block hash
	block.hash = block.calculateHash()
	
	return block
}

// calculateHash computes the block hash using Ethereum's method
func (b *EVMBlock) calculateHash() hotstuff.Hash {
	hasher := sha3.NewLegacyKeccak256()
	
	// Serialize header for hashing (similar to Ethereum)
	headerData := b.serializeHeader()
	hasher.Write(headerData)
	
	var hash hotstuff.Hash
	copy(hash[:], hasher.Sum(nil))
	return hash
}

// serializeHeader creates a byte representation of the header for hashing
func (b *EVMBlock) serializeHeader() []byte {
	var buf bytes.Buffer
	
	// Parent hash
	buf.Write(b.parent[:])
	
	// State root
	buf.Write(b.Header.StateRoot[:])
	
	// Transaction root
	buf.Write(b.Header.TxRoot[:])
	
	// Receipt root
	buf.Write(b.Header.ReceiptRoot[:])
	
	// Logs bloom
	buf.Write(b.Header.LogsBloom)
	
	// Gas limit and gas used
	binary.Write(&buf, binary.BigEndian, b.Header.GasLimit)
	binary.Write(&buf, binary.BigEndian, b.Header.GasUsed)
	
	// Timestamp
	binary.Write(&buf, binary.BigEndian, b.Header.Timestamp)
	
	// Block number
	if b.Header.Number != nil {
		buf.Write(b.Header.Number.Bytes())
	}
	
	// Coinbase
	buf.Write(b.Header.Coinbase[:])
	
	// Difficulty
	if b.Header.Difficulty != nil {
		buf.Write(b.Header.Difficulty.Bytes())
	}
	
	// Extra data
	buf.Write(b.Header.ExtraData)
	
	// Base fee
	if b.Header.BaseFee != nil {
		buf.Write(b.Header.BaseFee.Bytes())
	}
	
	return buf.Bytes()
}

// calculateTransactionRoot computes the Merkle root of transactions
func calculateTransactionRoot(transactions []*txpool.Transaction) hotstuff.Hash {
	if len(transactions) == 0 {
		return hotstuff.Hash{} // Empty root
	}
	
	// Simplified Merkle root calculation
	// In production, this should use proper Merkle Patricia Trie
	hasher := sha256.New()
	for _, tx := range transactions {
		hash := tx.Hash()
		hasher.Write(hash[:])
	}
	
	var root hotstuff.Hash
	copy(root[:], hasher.Sum(nil))
	return root
}

// Implement hotstuff.Block interface
func (b *EVMBlock) Hash() hotstuff.Hash {
	return b.hash
}

func (b *EVMBlock) Parent() hotstuff.Hash {
	return b.parent
}

func (b *EVMBlock) Proposer() hotstuff.ID {
	return b.proposer
}

func (b *EVMBlock) Command() hotstuff.Command {
	// Serialize transactions as command
	data, _ := json.Marshal(b.Transactions)
	return hotstuff.Command(data)
}

func (b *EVMBlock) QuorumCert() hotstuff.QuorumCert {
	return b.cert
}

func (b *EVMBlock) View() hotstuff.View {
	return b.view
}

func (b *EVMBlock) Timestamp() time.Time {
	return b.ts
}

func (b *EVMBlock) ToBytes() []byte {
	// Use Ethereum-style header serialization
	return b.serializeHeader()
}

// EVM-specific methods

// GetTransaction returns a transaction by index
func (b *EVMBlock) GetTransaction(index uint64) *txpool.Transaction {
	if index >= uint64(len(b.Transactions)) {
		return nil
	}
	return b.Transactions[index]
}

// GetReceipt returns a receipt by transaction index
func (b *EVMBlock) GetReceipt(index uint64) *TransactionReceipt {
	if index >= uint64(len(b.Receipts)) {
		return nil
	}
	return b.Receipts[index]
}

// TransactionCount returns the number of transactions in the block
func (b *EVMBlock) TransactionCount() int {
	return len(b.Transactions)
}

// UpdateReceipts updates the block's receipts and recalculates related fields
func (b *EVMBlock) UpdateReceipts(receipts []*TransactionReceipt) {
	b.Receipts = receipts
	
	// Recalculate receipt root
	b.Header.ReceiptRoot = b.calculateReceiptRoot()
	
	// Update gas used
	var totalGasUsed uint64
	for _, receipt := range receipts {
		totalGasUsed = receipt.CumulativeGasUsed
	}
	b.Header.GasUsed = totalGasUsed
	
	// Update logs bloom
	b.Header.LogsBloom = b.calculateLogsBloom()
	
	// Update size
	b.Header.Size = b.calculateSize()
	
	// Recalculate hash with updated header
	b.hash = b.calculateHash()
}

// calculateReceiptRoot computes the Merkle root of transaction receipts
func (b *EVMBlock) calculateReceiptRoot() hotstuff.Hash {
	if len(b.Receipts) == 0 {
		return hotstuff.Hash{}
	}
	
	// Simplified receipt root calculation
	hasher := sha256.New()
	for _, receipt := range b.Receipts {
		receiptData, _ := json.Marshal(receipt)
		hasher.Write(receiptData)
	}
	
	var root hotstuff.Hash
	copy(root[:], hasher.Sum(nil))
	return root
}

// calculateLogsBloom creates a bloom filter for all logs in the block
func (b *EVMBlock) calculateLogsBloom() []byte {
	bloom := make([]byte, 256)
	
	for _, receipt := range b.Receipts {
		for _, log := range receipt.Logs {
			// Add address to bloom
			addToBloom(bloom, log.Address[:])
			
			// Add topics to bloom
			for _, topic := range log.Topics {
				addToBloom(bloom, topic[:])
			}
		}
	}
	
	return bloom
}

// addToBloom adds data to the bloom filter (simplified implementation)
func addToBloom(bloom []byte, data []byte) {
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write(data)
	hash := hasher.Sum(nil)
	
	// Set 3 bits based on hash (simplified Ethereum bloom)
	for i := 0; i < 3; i++ {
		bit := binary.BigEndian.Uint16(hash[i*2:i*2+2]) % (256 * 8)
		bloom[bit/8] |= 1 << (bit % 8)
	}
}

// calculateSize estimates the block size in bytes
func (b *EVMBlock) calculateSize() uint64 {
	data, _ := json.Marshal(b)
	return uint64(len(data))
}

// String returns a string representation of the block
func (b *EVMBlock) String() string {
	return fmt.Sprintf(
		"EVMBlock{ hash: %.8s, number: %s, txs: %d, gasUsed: %d/%d, proposer: %d }",
		b.Hash().String(),
		b.Header.Number.String(),
		len(b.Transactions),
		b.Header.GasUsed,
		b.Header.GasLimit,
		b.proposer,
	)
}
