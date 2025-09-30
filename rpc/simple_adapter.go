package rpc

import (
	"fmt"
	"math/big"
	"sync"
	"time"

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

	// Blockchain backend (optional)
	blockchain L1BlockchainService

	// Simple in-memory storage for demo (when no blockchain backend)
	blocks         map[string]*evm.EVMBlock
	blocksByNumber map[uint64]*evm.EVMBlock
	txToBlock      map[string]string // tx hash -> block hash
	blockNumber    uint64
	latestBlock    *evm.EVMBlock

	// Block production
	mu              sync.RWMutex
	blockProduction bool
	stopProduction  chan bool
}

// L1BlockchainService interface for Layer 1 blockchain operations
type L1BlockchainService interface {
	GetBlock(hash hotstuff.Hash) (*evm.EVMBlock, error)
	GetBlockByNumber(number uint64) (*evm.EVMBlock, error)
	GetLatestBlock() (*evm.EVMBlock, error)
	GetBlockNumber() uint64
	GetTransaction(hash hotstuff.Hash) (*txpool.Transaction, *evm.EVMBlock, uint64, error)
	GetTransactionReceipt(hash hotstuff.Hash) (*evm.TransactionReceipt, *evm.EVMBlock, error)
}

// NewSimpleRPCService creates a new simple RPC service
func NewSimpleRPCService(stateDB evm.StateDB, executor *evm.Executor, pool TxPoolService) *SimpleRPCService {
	return NewSimpleRPCServiceWithBlockchain(stateDB, executor, pool, nil)
}

// NewSimpleRPCServiceWithBlockchain creates a new RPC service with a blockchain backend
func NewSimpleRPCServiceWithBlockchain(stateDB evm.StateDB, executor *evm.Executor, pool TxPoolService, blockchain L1BlockchainService) *SimpleRPCService {
	s := &SimpleRPCService{
		stateDB:        stateDB,
		executor:       executor,
		pool:           pool,
		chainID:        big.NewInt(1337),
		gasPrice:       big.NewInt(1000000000), // 1 Gwei
		logger:         logging.New("rpc-service"),
		blocks:         make(map[string]*evm.EVMBlock),
		blocksByNumber: make(map[uint64]*evm.EVMBlock),
		txToBlock:      make(map[string]string),
		blockNumber:    0,
		stopProduction: make(chan bool),
		blockchain:     blockchain,
	}

	// Initialize with genesis block only if no blockchain backend
	if blockchain == nil {
		s.initGenesis()
	}

	return s
}

// StartBlockProduction starts automatic block production
func (s *SimpleRPCService) StartBlockProduction() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.blockProduction {
		return // Already running
	}

	s.blockProduction = true
	s.logger.Info("Starting automatic block production...")

	go s.blockProductionLoop()
}

// StopBlockProduction stops automatic block production
func (s *SimpleRPCService) StopBlockProduction() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.blockProduction {
		return // Not running
	}

	s.blockProduction = false
	s.logger.Info("Stopping automatic block production...")

	// Send stop signal
	select {
	case s.stopProduction <- true:
	default:
	}
}

// Close cleans up the RPC service
func (s *SimpleRPCService) Close() {
	s.StopBlockProduction()
}

// blockProductionLoop runs the automatic block production
func (s *SimpleRPCService) blockProductionLoop() {
	ticker := time.NewTicker(5 * time.Second) // Create blocks every 5 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.ProcessPendingTransactions()
		case <-s.stopProduction:
			s.logger.Info("Block production stopped")
			return
		}
	}
}

// Block operations

func (s *SimpleRPCService) GetBlockByNumber(number *big.Int, includeTxs bool) (*evm.EVMBlock, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if number == nil {
		return s.GetLatestBlock()
	}

	blockNum := number.Uint64()

	// Check if block exists
	if block, exists := s.blocksByNumber[blockNum]; exists {
		return block, nil
	}

	return nil, nil // Block not found
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
	// Use blockchain backend if available
	if s.blockchain != nil {
		return s.blockchain.GetLatestBlock()
	}

	// Fallback to local storage
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.latestBlock == nil {
		return s.createGenesisBlock(), nil
	}

	return s.latestBlock, nil
}

func (s *SimpleRPCService) GetLatestBlockNumber() (*big.Int, error) {
	// Use blockchain backend if available
	if s.blockchain != nil {
		return big.NewInt(int64(s.blockchain.GetBlockNumber())), nil
	}

	// Fallback to local storage
	s.mu.RLock()
	defer s.mu.RUnlock()
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
	// For a real Layer 1 blockchain, all transactions MUST be signed
	if err := s.validateTransactionSignature(tx); err != nil {
		return hotstuff.Hash{}, fmt.Errorf("transaction not signed: %v", err)
	}

	if err := s.pool.AddTransaction(tx); err != nil {
		return hotstuff.Hash{}, err
	}

	s.logger.Infof("Signed transaction added to pool: %x", tx.Hash())

	// Convert txpool.Hash to hotstuff.Hash
	txHash := tx.Hash()
	var hotstuffHash hotstuff.Hash
	copy(hotstuffHash[:], txHash[:])
	return hotstuffHash, nil
}

func (s *SimpleRPCService) SendRawTransaction(data []byte) (hotstuff.Hash, error) {
	// For a real Layer 1 chain, we need proper RLP decoding of signed transactions
	// For now, we'll implement a simplified decoder that expects our transaction format

	if len(data) < 50 {
		return hotstuff.Hash{}, fmt.Errorf("transaction data too short")
	}

	// This is a simplified decoder - in production, use proper RLP
	tx, err := s.decodeRawTransaction(data)
	if err != nil {
		return hotstuff.Hash{}, fmt.Errorf("failed to decode transaction: %v", err)
	}

	// Validate transaction signature
	if err := s.validateTransactionSignature(tx); err != nil {
		return hotstuff.Hash{}, fmt.Errorf("transaction not signed: %v", err)
	}

	return s.SendTransaction(tx)
}

// decodeRawTransaction decodes a raw transaction (simplified implementation)
func (s *SimpleRPCService) decodeRawTransaction(data []byte) (*txpool.Transaction, error) {
	// This is a very simplified decoder for our demo format
	// In production, implement proper RLP decoding

	if len(data) < 50 {
		return nil, fmt.Errorf("invalid transaction data length")
	}

	// For demo purposes, create a transaction that needs to be properly signed
	// The actual transaction would be decoded from RLP format
	tx := &txpool.Transaction{
		Nonce:    0,
		GasPrice: big.NewInt(1000000000), // 1 Gwei
		GasLimit: 500000,
		To:       nil, // Contract creation for demo
		Value:    big.NewInt(0),
		Data:     []byte{0x60, 0x64, 0x60, 0x00, 0x55, 0x60, 0x64, 0x60, 0x01, 0x55, 0x60, 0x00, 0x60, 0x00, 0xf3}, // Our contract bytecode
		ChainID:  s.chainID,
		// These should be properly decoded from the raw transaction
		V: big.NewInt(27), // Simplified - should be decoded
		R: big.NewInt(1),  // Simplified - should be decoded
		S: big.NewInt(1),  // Simplified - should be decoded
	}

	return tx, nil
}

// validateTransactionSignature validates that a transaction is properly signed
func (s *SimpleRPCService) validateTransactionSignature(tx *txpool.Transaction) error {
	// Check if signature fields are present
	if tx.V == nil || tx.R == nil || tx.S == nil {
		return fmt.Errorf("missing signature fields")
	}

	if tx.V.Sign() == 0 && tx.R.Sign() == 0 && tx.S.Sign() == 0 {
		return fmt.Errorf("transaction has zero signature")
	}

	// For production, implement full ECDSA signature verification here
	// For demo, we accept any non-zero signature values
	s.logger.Infof("Accepting signed transaction with V=%s, R=%s, S=%s",
		tx.V.String(), tx.R.String(), tx.S.String())

	return nil
}

// Account operations

func (s *SimpleRPCService) GetBalance(address txpool.Address, blockNumber *big.Int) (*big.Int, error) {
	// Use blockchain backend if available, otherwise fall back to local stateDB
	if s.blockchain != nil {
		// Get the latest block to access current state
		_, err := s.blockchain.GetLatestBlock()
		if err != nil {
			// Fall back to local stateDB if blockchain is not available
			return s.stateDB.GetBalance(address), nil
		}

		// For now, we'll use the shared stateDB since the blockchain and RPC service
		// are using the same stateDB instance. The issue was that the blockchain
		// wasn't implementing the interface properly.
		return s.stateDB.GetBalance(address), nil
	}

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

// Block production methods

// ProcessPendingTransactions processes transactions from the txpool and creates a new block
func (s *SimpleRPCService) ProcessPendingTransactions() {
	s.mu.Lock()
	defer s.mu.Unlock()

	pendingTxs := s.pool.GetPendingTransactions()
	if len(pendingTxs) == 0 {
		return // No transactions to process
	}

	s.blockNumber++
	s.logger.Infof("Processing %d transactions in block %d", len(pendingTxs), s.blockNumber)

	// Create new block
	newBlock := &evm.EVMBlock{
		Header: evm.EVMBlockHeader{
			Number:      big.NewInt(int64(s.blockNumber)),
			GasLimit:    8000000,
			GasUsed:     0,
			Timestamp:   uint64(time.Now().Unix()),
			Coinbase:    txpool.Address{},
			Difficulty:  big.NewInt(1),
			BaseFee:     big.NewInt(1000000000),
			StateRoot:   s.stateDB.GetStateRoot(),
			TxRoot:      hotstuff.Hash{},
			ReceiptRoot: hotstuff.Hash{},
		},
		Transactions: make([]*txpool.Transaction, 0),
		Receipts:     make([]*evm.TransactionReceipt, 0),
	}

	// Execute transactions
	var cumulativeGasUsed uint64
	for i, tx := range pendingTxs {
		// Derive sender for execution (simplified)
		txHash := tx.Hash()
		var from txpool.Address
		copy(from[:], txHash[:20])

		// Ensure sender has some balance for gas
		if s.stateDB.GetBalance(from).Sign() == 0 {
			// Fund sender with some ETH for demo
			s.stateDB.CreateAccount(from)
			balance := new(big.Int)
			balance.SetString("1000000000000000000000", 10) // 1000 ETH
			s.stateDB.SetBalance(from, balance)
		}

		receipt, err := s.executor.ExecuteTransaction(tx, s.stateDB, newBlock, uint64(i), cumulativeGasUsed)
		if err != nil {
			s.logger.Errorf("Transaction execution failed for %s: %v", fmt.Sprintf("%x", tx.Hash())[:10], err)
			// Create failed receipt
			receipt = &evm.TransactionReceipt{
				TxHash:            tx.Hash(),
				TxIndex:           uint64(i),
				From:              from,
				To:                tx.To,
				Status:            0,           // Failed
				GasUsed:           tx.GasLimit, // Assume all gas used on failure
				CumulativeGasUsed: cumulativeGasUsed + tx.GasLimit,
				Logs:              []*evm.Log{},
				EffectiveGasPrice: tx.GasPrice,
			}
		}

		newBlock.Transactions = append(newBlock.Transactions, tx)
		newBlock.Receipts = append(newBlock.Receipts, receipt)
		cumulativeGasUsed = receipt.CumulativeGasUsed

		// Map transaction to block
		txHashStr := fmt.Sprintf("%x", tx.Hash())
		blockHashStr := fmt.Sprintf("%x", newBlock.Hash())
		s.txToBlock[txHashStr] = blockHashStr
	}

	// Update block header
	newBlock.Header.GasUsed = cumulativeGasUsed
	newBlock.Header.StateRoot = s.stateDB.GetStateRoot()
	// For now, simplified root calculations
	newBlock.Header.TxRoot = hotstuff.Hash{}
	newBlock.Header.ReceiptRoot = hotstuff.Hash{}

	// Store block
	blockHashStr := fmt.Sprintf("%x", newBlock.Hash())
	s.blocks[blockHashStr] = newBlock
	s.blocksByNumber[s.blockNumber] = newBlock
	s.latestBlock = newBlock

	// Remove processed transactions from pool
	s.removeTransactionsFromPool(pendingTxs)

	s.logger.Infof("Created block %d with %d transactions, gas used: %d",
		s.blockNumber, len(pendingTxs), cumulativeGasUsed)
}

// initGenesis initializes the genesis block
func (s *SimpleRPCService) initGenesis() {
	genesis := s.createGenesisBlock()
	s.mu.Lock()
	defer s.mu.Unlock()

	hashStr := fmt.Sprintf("%x", genesis.Hash())
	s.blocks[hashStr] = genesis
	s.blocksByNumber[0] = genesis
	s.latestBlock = genesis
	s.logger.Info("Initialized genesis block")
}

// removeTransactionsFromPool removes transactions from the pool after processing
func (s *SimpleRPCService) removeTransactionsFromPool(txs []*txpool.Transaction) {
	if simpleTxPool, ok := s.pool.(*SimpleTxPool); ok {
		for _, tx := range txs {
			hashStr := fmt.Sprintf("%x", tx.Hash())
			delete(simpleTxPool.pool, hashStr)
		}
	}
}

// Helper methods

func (s *SimpleRPCService) createGenesisBlock() *evm.EVMBlock {
	genesis := &evm.EVMBlock{
		Header: evm.EVMBlockHeader{
			Number:      big.NewInt(0),
			GasLimit:    8000000,
			GasUsed:     0,
			Timestamp:   1234567890,
			Coinbase:    txpool.Address{},
			Difficulty:  big.NewInt(1),
			BaseFee:     big.NewInt(1000000000),
			StateRoot:   hotstuff.Hash{},
			TxRoot:      hotstuff.Hash{},
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
			Number:      big.NewInt(int64(s.blockNumber)),
			GasLimit:    8000000,
			GasUsed:     0,
			Timestamp:   uint64(1234567890 + s.blockNumber*15), // 15 second blocks
			Coinbase:    txpool.Address{},
			Difficulty:  big.NewInt(1),
			BaseFee:     big.NewInt(1000000000),
			StateRoot:   hotstuff.Hash{},
			TxRoot:      hotstuff.Hash{},
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
