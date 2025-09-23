package rpc

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/evm"
	"github.com/relab/hotstuff/txpool"
)

// Ethereum JSON-RPC types and responses

// HexNumber represents a hex-encoded number
type HexNumber string

// NewHexNumber creates a hex number from uint64
func NewHexNumber(n uint64) HexNumber {
	return HexNumber("0x" + strconv.FormatUint(n, 16))
}

// NewHexNumberFromBig creates a hex number from big.Int
func NewHexNumberFromBig(n *big.Int) HexNumber {
	if n == nil {
		return "0x0"
	}
	return HexNumber("0x" + n.Text(16))
}

// ToUint64 converts hex number to uint64
func (h HexNumber) ToUint64() (uint64, error) {
	if h == "" || h == "0x" {
		return 0, nil
	}
	return strconv.ParseUint(string(h)[2:], 16, 64)
}

// ToBig converts hex number to big.Int
func (h HexNumber) ToBig() (*big.Int, error) {
	if h == "" || h == "0x" {
		return big.NewInt(0), nil
	}
	result := new(big.Int)
	result.SetString(string(h)[2:], 16)
	return result, nil
}

// HexBytes represents hex-encoded bytes
type HexBytes string

// NewHexBytes creates hex bytes from byte slice
func NewHexBytes(data []byte) HexBytes {
	if len(data) == 0 {
		return "0x"
	}
	return HexBytes("0x" + fmt.Sprintf("%x", data))
}

// ToBytes converts hex bytes to byte slice
func (h HexBytes) ToBytes() ([]byte, error) {
	if h == "" || h == "0x" {
		return []byte{}, nil
	}
	// Remove 0x prefix and decode
	hexStr := string(h)[2:]
	if len(hexStr)%2 != 0 {
		hexStr = "0" + hexStr
	}
	
	result := make([]byte, len(hexStr)/2)
	for i := 0; i < len(hexStr); i += 2 {
		b, err := strconv.ParseUint(hexStr[i:i+2], 16, 8)
		if err != nil {
			return nil, err
		}
		result[i/2] = byte(b)
	}
	return result, nil
}

// Address represents an Ethereum address
type Address string

// NewAddress creates an address from txpool.Address
func NewAddress(addr txpool.Address) Address {
	return Address("0x" + fmt.Sprintf("%040x", addr))
}

// ToTxpoolAddress converts to txpool.Address
func (a Address) ToTxpoolAddress() (txpool.Address, error) {
	if len(a) != 42 { // 0x + 40 hex chars
		return txpool.Address{}, fmt.Errorf("invalid address length")
	}
	
	var addr txpool.Address
	_, err := fmt.Sscanf(string(a), "0x%040x", &addr)
	return addr, err
}

// Hash represents a hash value
type Hash string

// NewHash creates a hash from hotstuff.Hash
func NewHash(hash hotstuff.Hash) Hash {
	return Hash("0x" + fmt.Sprintf("%064x", hash))
}

// ToHotstuffHash converts to hotstuff.Hash
func (h Hash) ToHotstuffHash() (hotstuff.Hash, error) {
	if len(h) != 66 { // 0x + 64 hex chars
		return hotstuff.Hash{}, fmt.Errorf("invalid hash length")
	}
	
	var hash hotstuff.Hash
	_, err := fmt.Sscanf(string(h), "0x%064x", &hash)
	return hash, err
}

// Block represents an Ethereum block for JSON-RPC
type Block struct {
	Number           HexNumber    `json:"number"`
	Hash             Hash         `json:"hash"`
	ParentHash       Hash         `json:"parentHash"`
	Nonce            HexBytes     `json:"nonce"`
	Sha3Uncles       Hash         `json:"sha3Uncles"`
	LogsBloom        HexBytes     `json:"logsBloom"`
	TransactionsRoot Hash         `json:"transactionsRoot"`
	StateRoot        Hash         `json:"stateRoot"`
	ReceiptsRoot     Hash         `json:"receiptsRoot"`
	Miner            Address      `json:"miner"`
	Difficulty       HexNumber    `json:"difficulty"`
	TotalDifficulty  HexNumber    `json:"totalDifficulty"`
	ExtraData        HexBytes     `json:"extraData"`
	Size             HexNumber    `json:"size"`
	GasLimit         HexNumber    `json:"gasLimit"`
	GasUsed          HexNumber    `json:"gasUsed"`
	Timestamp        HexNumber    `json:"timestamp"`
	Transactions     []Hash       `json:"transactions"`
	Uncles           []Hash       `json:"uncles"`
	BaseFeePerGas    *HexNumber   `json:"baseFeePerGas,omitempty"`
}

// NewBlockFromEVM creates a Block from evm.EVMBlock
func NewBlockFromEVM(block *evm.EVMBlock, includeTxs bool) *Block {
	txHashes := make([]Hash, len(block.Transactions))
	for i, tx := range block.Transactions {
		txHash := tx.Hash()
		var hotstuffHash hotstuff.Hash
		copy(hotstuffHash[:], txHash[:])
		txHashes[i] = NewHash(hotstuffHash)
	}
	
	result := &Block{
		Number:           NewHexNumberFromBig(block.Header.Number),
		Hash:             NewHash(block.Hash()),
		ParentHash:       NewHash(block.Parent()), // Use HotStuff parent
		Nonce:            "0x0000000000000000", // Not used in our consensus
		Sha3Uncles:       NewHash(hotstuff.Hash{}), // No uncles in HotStuff
		LogsBloom:        NewHexBytes(make([]byte, 256)), // Simplified
		TransactionsRoot: NewHash(block.Header.TxRoot),
		StateRoot:        NewHash(block.Header.StateRoot),
		ReceiptsRoot:     NewHash(block.Header.ReceiptRoot),
		Miner:            NewAddress(block.Header.Coinbase),
		Difficulty:       "0x1", // Fixed difficulty for HotStuff
		TotalDifficulty:  NewHexNumberFromBig(block.Header.Number), // Simplified
		ExtraData:        "0x486f7453747566662d45564d", // "HotStuff-EVM" in hex
		Size:             NewHexNumber(uint64(len(block.ToBytes()))), // Approximate size
		GasLimit:         NewHexNumber(block.Header.GasLimit),
		GasUsed:          NewHexNumber(block.Header.GasUsed),
		Timestamp:        NewHexNumber(block.Header.Timestamp),
		Transactions:     txHashes,
		Uncles:           []Hash{}, // No uncles in HotStuff
	}
	
	if block.Header.BaseFee != nil {
		baseFee := NewHexNumberFromBig(block.Header.BaseFee)
		result.BaseFeePerGas = &baseFee
	}
	
	return result
}

// Transaction represents an Ethereum transaction for JSON-RPC
type Transaction struct {
	Hash             Hash        `json:"hash"`
	Nonce            HexNumber   `json:"nonce"`
	BlockHash        *Hash       `json:"blockHash"`
	BlockNumber      *HexNumber  `json:"blockNumber"`
	TransactionIndex *HexNumber  `json:"transactionIndex"`
	From             Address     `json:"from"`
	To               *Address    `json:"to"`
	Value            HexNumber   `json:"value"`
	GasPrice         HexNumber   `json:"gasPrice"`
	Gas              HexNumber   `json:"gas"`
	Input            HexBytes    `json:"input"`
	V                HexNumber   `json:"v"`
	R                HexNumber   `json:"r"`
	S                HexNumber   `json:"s"`
	Type             *HexNumber  `json:"type,omitempty"`
	ChainId          *HexNumber  `json:"chainId,omitempty"`
}

// NewTransactionFromTxpool creates a Transaction from txpool.Transaction
func NewTransactionFromTxpool(tx *txpool.Transaction, blockHash *hotstuff.Hash, blockNumber *big.Int, txIndex *uint64) *Transaction {
	// Derive sender (simplified)
	hash := tx.Hash()
	var from txpool.Address
	copy(from[:], hash[:20])
	
	// Convert txpool.Hash to hotstuff.Hash
	txHash := tx.Hash()
	var hotstuffHash hotstuff.Hash
	copy(hotstuffHash[:], txHash[:])
	
	result := &Transaction{
		Hash:      NewHash(hotstuffHash),
		Nonce:     NewHexNumber(tx.Nonce),
		From:      NewAddress(from),
		Value:     NewHexNumberFromBig(tx.Value),
		GasPrice:  NewHexNumberFromBig(tx.GasPrice),
		Gas:       NewHexNumber(tx.GasLimit),
		Input:     NewHexBytes(tx.Data),
		V:         NewHexNumberFromBig(tx.V),
		R:         NewHexNumberFromBig(tx.R),
		S:         NewHexNumberFromBig(tx.S),
	}
	
	if tx.To != nil {
		to := NewAddress(*tx.To)
		result.To = &to
	}
	
	if blockHash != nil {
		hash := NewHash(*blockHash)
		result.BlockHash = &hash
	}
	
	if blockNumber != nil {
		num := NewHexNumberFromBig(blockNumber)
		result.BlockNumber = &num
	}
	
	if txIndex != nil {
		idx := NewHexNumber(*txIndex)
		result.TransactionIndex = &idx
	}
	
	if tx.ChainID != nil {
		chainId := NewHexNumberFromBig(tx.ChainID)
		result.ChainId = &chainId
	}
	
	return result
}

// TransactionReceipt represents an Ethereum transaction receipt for JSON-RPC
type TransactionReceipt struct {
	TransactionHash   Hash       `json:"transactionHash"`
	TransactionIndex  HexNumber  `json:"transactionIndex"`
	BlockHash         Hash       `json:"blockHash"`
	BlockNumber       HexNumber  `json:"blockNumber"`
	From              Address    `json:"from"`
	To                *Address   `json:"to"`
	CumulativeGasUsed HexNumber  `json:"cumulativeGasUsed"`
	GasUsed           HexNumber  `json:"gasUsed"`
	ContractAddress   *Address   `json:"contractAddress"`
	Logs              []Log      `json:"logs"`
	LogsBloom         HexBytes   `json:"logsBloom"`
	Status            HexNumber  `json:"status"`
	EffectiveGasPrice HexNumber  `json:"effectiveGasPrice"`
	Type              HexNumber  `json:"type"`
}

// Log represents an Ethereum log for JSON-RPC
type Log struct {
	Address          Address    `json:"address"`
	Topics           []Hash     `json:"topics"`
	Data             HexBytes   `json:"data"`
	BlockNumber      HexNumber  `json:"blockNumber"`
	TransactionHash  Hash       `json:"transactionHash"`
	TransactionIndex HexNumber  `json:"transactionIndex"`
	BlockHash        Hash       `json:"blockHash"`
	LogIndex         HexNumber  `json:"logIndex"`
	Removed          bool       `json:"removed"`
}

// NewTransactionReceiptFromEVM creates a TransactionReceipt from evm.TransactionReceipt
func NewTransactionReceiptFromEVM(receipt *evm.TransactionReceipt, blockHash hotstuff.Hash, blockNumber *big.Int) *TransactionReceipt {
	logs := make([]Log, len(receipt.Logs))
	for i, log := range receipt.Logs {
		topics := make([]Hash, len(log.Topics))
		for j, topic := range log.Topics {
			topics[j] = NewHash(topic)
		}
		
		// Convert log TxHash
		logTxHash := receipt.TxHash
		var hotstuffLogTxHash hotstuff.Hash
		copy(hotstuffLogTxHash[:], logTxHash[:])
		
		logs[i] = Log{
			Address:          NewAddress(log.Address),
			Topics:           topics,
			Data:             NewHexBytes(log.Data),
			BlockNumber:      NewHexNumberFromBig(blockNumber),
			TransactionHash:  NewHash(hotstuffLogTxHash),
			TransactionIndex: NewHexNumber(receipt.TxIndex),
			BlockHash:        NewHash(blockHash),
			LogIndex:         NewHexNumber(uint64(i)),
			Removed:          false,
		}
	}
	
	// Convert receipt TxHash
	receiptTxHash := receipt.TxHash
	var hotstuffTxHash hotstuff.Hash
	copy(hotstuffTxHash[:], receiptTxHash[:])
	
	result := &TransactionReceipt{
		TransactionHash:   NewHash(hotstuffTxHash),
		TransactionIndex:  NewHexNumber(receipt.TxIndex),
		BlockHash:         NewHash(blockHash),
		BlockNumber:       NewHexNumberFromBig(blockNumber),
		From:              NewAddress(receipt.From),
		CumulativeGasUsed: NewHexNumber(receipt.CumulativeGasUsed),
		GasUsed:           NewHexNumber(receipt.GasUsed),
		Logs:              logs,
		LogsBloom:         NewHexBytes(receipt.LogsBloom),
		Status:            NewHexNumber(receipt.Status),
		EffectiveGasPrice: NewHexNumberFromBig(receipt.EffectiveGasPrice),
		Type:              "0x0", // Legacy transaction type
	}
	
	if receipt.To != nil {
		to := NewAddress(*receipt.To)
		result.To = &to
	}
	
	if receipt.ContractAddress != nil {
		addr := NewAddress(*receipt.ContractAddress)
		result.ContractAddress = &addr
	}
	
	return result
}

// CallArgs represents arguments for eth_call
type CallArgs struct {
	From     *Address   `json:"from"`
	To       *Address   `json:"to"`
	Gas      *HexNumber `json:"gas"`
	GasPrice *HexNumber `json:"gasPrice"`
	Value    *HexNumber `json:"value"`
	Data     *HexBytes  `json:"data"`
}

// ToTxpoolTransaction converts CallArgs to txpool.Transaction
func (args *CallArgs) ToTxpoolTransaction() (*txpool.Transaction, error) {
	tx := &txpool.Transaction{
		Nonce:    0, // Will be set by the caller
		Value:    big.NewInt(0),
		GasLimit: 21000, // Default gas limit
		GasPrice: big.NewInt(1000000000), // 1 Gwei default
		Data:     []byte{},
		ChainID:  big.NewInt(1337), // Default chain ID
	}
	
	if args.Value != nil {
		value, err := args.Value.ToBig()
		if err != nil {
			return nil, err
		}
		tx.Value = value
	}
	
	if args.Gas != nil {
		gas, err := args.Gas.ToUint64()
		if err != nil {
			return nil, err
		}
		tx.GasLimit = gas
	}
	
	if args.GasPrice != nil {
		gasPrice, err := args.GasPrice.ToBig()
		if err != nil {
			return nil, err
		}
		tx.GasPrice = gasPrice
	}
	
	if args.Data != nil {
		data, err := args.Data.ToBytes()
		if err != nil {
			return nil, err
		}
		tx.Data = data
	}
	
	if args.To != nil {
		to, err := args.To.ToTxpoolAddress()
		if err != nil {
			return nil, err
		}
		tx.To = &to
	}
	
	return tx, nil
}

// JSONRPCRequest represents a JSON-RPC 2.0 request
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      interface{}     `json:"id"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

// RPCError represents a JSON-RPC error
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Error implements the error interface
func (e *RPCError) Error() string {
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

// Standard JSON-RPC error codes
const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

// NewRPCError creates a new RPC error
func NewRPCError(code int, message string, data interface{}) *RPCError {
	return &RPCError{
		Code:    code,
		Message: message,
		Data:    data,
	}
}
