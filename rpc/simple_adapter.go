package rpc

import (
	"fmt"
	"math/big"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/evm"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/txpool"
)

// SimpleRPCService implements the RPC Service interface with minimal functionality
type SimpleRPCService struct {
	stateDB  evm.StateDB
	executor *evm.Executor
	pool     TxPoolService
	chainID  *big.Int
	gasPrice *big.Int
	logger   logging.Logger
	
	// Simple in-memory storage for demo
	blocks      map[string]*evm.EVMBlock
	txToBlock   map[string]string // tx hash -> block hash
	blockNumber uint64
}

// NewSimpleRPCService creates a new simple RPC service
func NewSimpleRPCService(stateDB evm.StateDB, executor *evm.Executor, pool TxPoolService) *SimpleRPCService {
	return &SimpleRPCService{
		stateDB:     stateDB,
		executor:    executor,
		pool:        pool,
		chainID:     big.NewInt(1337),
		gasPrice:    big.NewInt(1000000000), // 1 Gwei
		logger:      logging.New("rpc-service"),
		blocks:      make(map[string]*evm.EVMBlock),
		txToBlock:   make(map[string]string),
		blockNumber: 0,
	}
}

// Block operations

func (s *SimpleRPCService) GetBlockByNumber(number *big.Int, includeTxs bool) (*evm.EVMBlock, error) {
	if number == nil {
		return s.GetLatestBlock()
	}
	
	blockNum := number.Uint64()
	if blockNum == 0 {
		// Genesis block
		return s.createGenesisBlock(), nil
	}
	
	// For now, only return genesis or latest
	if blockNum > s.blockNumber {
		return nil, nil // Block not found
	}
	
	return s.GetLatestBlock()
}

func (s *SimpleRPCService) GetBlockByHash(hash hotstuff.Hash, includeTxs bool) (*evm.EVMBlock, error) {
	hashStr := fmt.Sprintf("%x", hash)
	block, exists := s.blocks[hashStr]
	if !exists {
		return nil, nil
	}
	return block, nil
}

func (s *SimpleRPCService) GetLatestBlock() (*evm.EVMBlock, error) {
	if s.blockNumber == 0 {
		return s.createGenesisBlock(), nil
	}
	
	// Create a sample block
	return s.createSampleBlock(), nil
}

func (s *SimpleRPCService) GetLatestBlockNumber() (*big.Int, error) {
	return big.NewInt(int64(s.blockNumber)), nil
}

// Transaction operations

func (s *SimpleRPCService) GetTransactionByHash(hash hotstuff.Hash) (*txpool.Transaction, *evm.EVMBlock, uint64, error) {
	// Check pool first
	hashStr := fmt.Sprintf("%x", hash)
	
	// Simple search in pending transactions
	pending := s.pool.GetPendingTransactions()
	for _, tx := range pending {
		txHash := tx.Hash()
		txHashStr := fmt.Sprintf("%x", txHash)
		if txHashStr == hashStr {
			return tx, nil, 0, nil // Pending transaction
		}
	}
	
	// Check if transaction is in a block
	_, exists := s.txToBlock[hashStr]
	if exists {
		block, err := s.GetBlockByHash(hotstuff.Hash{}, false)
		if err != nil {
			return nil, nil, 0, err
		}
		
		// Find transaction in block
		for i, tx := range block.Transactions {
			txHash := tx.Hash()
			txHashStr := fmt.Sprintf("%x", txHash)
			if txHashStr == hashStr {
				return tx, block, uint64(i), nil
			}
		}
	}
	
	return nil, nil, 0, nil
}

func (s *SimpleRPCService) GetTransactionReceipt(hash hotstuff.Hash) (*evm.TransactionReceipt, *evm.EVMBlock, error) {
	// Find transaction first
	_, block, txIndex, err := s.GetTransactionByHash(hash)
	if err != nil || block == nil {
		return nil, nil, err
	}
	
	// Return receipt if exists
	if int(txIndex) < len(block.Receipts) {
		return block.Receipts[txIndex], block, nil
	}
	
	return nil, nil, nil
}

func (s *SimpleRPCService) SendTransaction(tx *txpool.Transaction) (hotstuff.Hash, error) {
	if err := s.pool.AddTransaction(tx); err != nil {
		return hotstuff.Hash{}, err
	}
	// Convert txpool.Hash to hotstuff.Hash
	txHash := tx.Hash()
	var hotstuffHash hotstuff.Hash
	copy(hotstuffHash[:], txHash[:])
	return hotstuffHash, nil
}

func (s *SimpleRPCService) SendRawTransaction(data []byte) (hotstuff.Hash, error) {
	// Simplified raw transaction parsing
	tx := &txpool.Transaction{
		Nonce:    0,
		GasPrice: s.gasPrice,
		GasLimit: 21000,
		Value:    big.NewInt(0),
		Data:     data,
		ChainID:  s.chainID,
	}
	
	return s.SendTransaction(tx)
}

// Account operations

func (s *SimpleRPCService) GetBalance(address txpool.Address, blockNumber *big.Int) (*big.Int, error) {
	return s.stateDB.GetBalance(address), nil
}

func (s *SimpleRPCService) GetTransactionCount(address txpool.Address, blockNumber *big.Int) (uint64, error) {
	return s.stateDB.GetNonce(address), nil
}

func (s *SimpleRPCService) GetCode(address txpool.Address, blockNumber *big.Int) ([]byte, error) {
	return s.stateDB.GetCode(address), nil
}

func (s *SimpleRPCService) GetStorageAt(address txpool.Address, position hotstuff.Hash, blockNumber *big.Int) (hotstuff.Hash, error) {
	return s.stateDB.GetState(address, position), nil
}

// Call operations

func (s *SimpleRPCService) Call(args CallArgs, blockNumber *big.Int) ([]byte, error) {
	// Convert to transaction
	tx, err := args.ToTxpoolTransaction()
	if err != nil {
		return nil, err
	}
	
	// Create simple EVM for execution
	simpleEVM := evm.NewSimpleEVM(s.stateDB)
	
	if tx.To == nil {
		// Contract creation
		ret, _, err := simpleEVM.ExecuteContract(
			txpool.Address{}, // Zero address for calls
			txpool.Address{}, // Will be generated
			tx.Data,
			tx.Value,
			tx.GasLimit,
			true, // isCreate
		)
		return ret, err
	} else {
		// Contract call
		ret, _, err := simpleEVM.ExecuteContract(
			txpool.Address{}, // Zero address for calls
			*tx.To,
			tx.Data,
			tx.Value,
			tx.GasLimit,
			false, // isCreate
		)
		return ret, err
	}
}

func (s *SimpleRPCService) EstimateGas(args CallArgs) (uint64, error) {
	if args.Gas != nil {
		gas, err := args.Gas.ToUint64()
		if err == nil {
			return gas, nil
		}
	}
	
	// Default estimates
	if args.To == nil {
		return 200000, nil // Contract creation
	}
	if args.Data != nil && len(*args.Data) > 2 {
		return 100000, nil // Contract call
	}
	return 21000, nil // Simple transfer
}

// Network operations

func (s *SimpleRPCService) ChainID() *big.Int {
	return new(big.Int).Set(s.chainID)
}

func (s *SimpleRPCService) GasPrice() *big.Int {
	return new(big.Int).Set(s.gasPrice)
}

// Utility operations

func (s *SimpleRPCService) GetLogs(filter LogFilter) ([]evm.Log, error) {
	// Simplified log filtering
	var logs []evm.Log
	
	// For now, return empty logs
	// In a full implementation, we'd search through block receipts
	return logs, nil
}

// Helper methods

func (s *SimpleRPCService) createGenesisBlock() *evm.EVMBlock {
	genesis := &evm.EVMBlock{
		Header: evm.EVMBlockHeader{
			Number:    big.NewInt(0),
			GasLimit:  8000000,
			GasUsed:   0,
			Timestamp: 1234567890,
			Coinbase:  txpool.Address{},
			Difficulty: big.NewInt(1),
			BaseFee:   big.NewInt(1000000000),
			StateRoot: hotstuff.Hash{},
			TxRoot:    hotstuff.Hash{},
			ReceiptRoot: hotstuff.Hash{},
		},
		Transactions: []*txpool.Transaction{},
		Receipts:     []*evm.TransactionReceipt{},
	}
	
	// Set HotStuff fields (simplified)
	// genesis.SetView(0)    // Method doesn't exist in current implementation
	// genesis.SetProposer(1) // Method doesn't exist in current implementation
	
	return genesis
}

func (s *SimpleRPCService) createSampleBlock() *evm.EVMBlock {
	s.blockNumber++
	
	block := &evm.EVMBlock{
		Header: evm.EVMBlockHeader{
			Number:    big.NewInt(int64(s.blockNumber)),
			GasLimit:  8000000,
			GasUsed:   0,
			Timestamp: uint64(1234567890 + s.blockNumber*15), // 15 second blocks
			Coinbase:  txpool.Address{},
			Difficulty: big.NewInt(1),
			BaseFee:   big.NewInt(1000000000),
			StateRoot: hotstuff.Hash{},
			TxRoot:    hotstuff.Hash{},
			ReceiptRoot: hotstuff.Hash{},
		},
		Transactions: []*txpool.Transaction{},
		Receipts:     []*evm.TransactionReceipt{},
	}
	
	// Set HotStuff fields (simplified)
	// block.SetView(hotstuff.View(s.blockNumber)) // Method doesn't exist in current implementation
	// block.SetProposer(1) // Method doesn't exist in current implementation
	
	// Store block
	hashStr := fmt.Sprintf("%x", block.Hash())
	s.blocks[hashStr] = block
	
	return block
}

// Simple TxPool implementation for demo
type SimpleTxPool struct {
	pool map[string]*txpool.Transaction
}

func NewSimpleTxPool() *SimpleTxPool {
	return &SimpleTxPool{
		pool: make(map[string]*txpool.Transaction),
	}
}

func (p *SimpleTxPool) AddTransaction(tx *txpool.Transaction) error {
	hashStr := fmt.Sprintf("%x", tx.Hash())
	p.pool[hashStr] = tx
	return nil
}

func (p *SimpleTxPool) GetTransaction(hash hotstuff.Hash) (*txpool.Transaction, error) {
	hashStr := fmt.Sprintf("%x", hash)
	tx, exists := p.pool[hashStr]
	if !exists {
		return nil, fmt.Errorf("transaction not found")
	}
	return tx, nil
}

func (p *SimpleTxPool) GetPendingTransactions() []*txpool.Transaction {
	var txs []*txpool.Transaction
	for _, tx := range p.pool {
		txs = append(txs, tx)
	}
	return txs
}
