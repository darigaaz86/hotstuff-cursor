package evm

import (
	"fmt"
	"math/big"

	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/txpool"
)

// ExecutionConfig holds configuration for transaction execution
type ExecutionConfig struct {
	GasLimit uint64   // Block gas limit
	BaseFee  *big.Int // EIP-1559 base fee
	ChainID  *big.Int // Chain ID for replay protection
}

// Executor handles transaction execution and state transitions
type Executor struct {
	config ExecutionConfig
	logger logging.Logger
}

// NewExecutor creates a new transaction executor
func NewExecutor(config ExecutionConfig) *Executor {
	return &Executor{
		config: config,
		logger: logging.New("evm-executor"),
	}
}

// ExecuteBlock executes all transactions in a block and returns receipts
func (e *Executor) ExecuteBlock(block *EVMBlock, stateDB StateDB) ([]*TransactionReceipt, error) {
	receipts := make([]*TransactionReceipt, 0, len(block.Transactions))
	var cumulativeGasUsed uint64

	e.logger.Infof("Executing block with %d transactions", len(block.Transactions))

	for i, tx := range block.Transactions {
		receipt, err := e.ExecuteTransaction(tx, stateDB, block, uint64(i), cumulativeGasUsed)
		if err != nil {
			e.logger.Errorf("Failed to execute transaction %d: %v", i, err)
			// Create failed receipt
			receipt = e.createFailedReceipt(tx, block, uint64(i), cumulativeGasUsed, err)
		}

		receipts = append(receipts, receipt)
		cumulativeGasUsed = receipt.CumulativeGasUsed

		// Check block gas limit
		if cumulativeGasUsed > e.config.GasLimit {
			return nil, fmt.Errorf("block gas limit exceeded: %d > %d", cumulativeGasUsed, e.config.GasLimit)
		}
	}

	e.logger.Infof("Block execution completed, gas used: %d/%d", cumulativeGasUsed, e.config.GasLimit)
	return receipts, nil
}

// ExecuteTransaction executes a single transaction
func (e *Executor) ExecuteTransaction(tx *txpool.Transaction, stateDB StateDB,
	block *EVMBlock, txIndex uint64, cumulativeGasUsed uint64) (*TransactionReceipt, error) {

	// Take a snapshot for potential revert
	snapshot := stateDB.Snapshot()

	// Get sender address
	from, err := e.getSender(tx)
	if err != nil {
		stateDB.RevertToSnapshot(snapshot)
		return nil, fmt.Errorf("failed to get sender: %w", err)
	}

	// Pre-execution validation
	if err := e.validateTransaction(tx, stateDB, from); err != nil {
		stateDB.RevertToSnapshot(snapshot)
		return nil, fmt.Errorf("transaction validation failed: %w", err)
	}

	// Apply transaction
	receipt, err := e.applyTransaction(tx, stateDB, block, txIndex, cumulativeGasUsed, from)
	if err != nil {
		stateDB.RevertToSnapshot(snapshot)
		return nil, err
	}

	return receipt, nil
}

// getSender extracts the sender address from transaction signature
func (e *Executor) getSender(tx *txpool.Transaction) (txpool.Address, error) {
	// Simplified sender recovery - in production this would use ECDSA recovery
	// For now, derive from transaction hash for demo purposes
	hash := tx.Hash()
	var addr txpool.Address
	copy(addr[:], hash[:20])
	return addr, nil
}

// validateTransaction performs pre-execution validation
func (e *Executor) validateTransaction(tx *txpool.Transaction, stateDB StateDB, from txpool.Address) error {
	// Check basic transaction validity
	if err := tx.Validate(); err != nil {
		return err
	}

	// Check nonce
	accountNonce := stateDB.GetNonce(from)
	if tx.Nonce != accountNonce {
		return fmt.Errorf("invalid nonce: expected %d, got %d", accountNonce, tx.Nonce)
	}

	// Check balance for value + gas
	balance := stateDB.GetBalance(from)
	cost := new(big.Int).Mul(tx.GasPrice, big.NewInt(int64(tx.GasLimit)))
	cost = cost.Add(cost, tx.Value)

	if balance.Cmp(cost) < 0 {
		return fmt.Errorf("insufficient balance: need %s, have %s", cost.String(), balance.String())
	}

	// Check gas price against base fee (EIP-1559 simplified)
	if tx.GasPrice.Cmp(e.config.BaseFee) < 0 {
		return fmt.Errorf("gas price too low: %s < %s", tx.GasPrice.String(), e.config.BaseFee.String())
	}

	return nil
}

// applyTransaction applies the transaction to the state
func (e *Executor) applyTransaction(tx *txpool.Transaction, stateDB StateDB,
	block *EVMBlock, txIndex uint64, cumulativeGasUsed uint64, from txpool.Address) (*TransactionReceipt, error) {

	// Calculate effective gas price (simplified EIP-1559)
	effectiveGasPrice := tx.GasPrice

	// Deduct gas cost upfront
	gasCost := new(big.Int).Mul(effectiveGasPrice, big.NewInt(int64(tx.GasLimit)))
	stateDB.SubBalance(from, gasCost)

	// Increment nonce
	stateDB.SetNonce(from, stateDB.GetNonce(from)+1)

	var (
		gasUsed         uint64
		contractAddress *txpool.Address
		logs            []*Log
		err             error
	)

	if tx.To == nil {
		// Contract creation
		contractAddress, gasUsed, logs, err = e.createContract(tx, stateDB, from)
	} else {
		// Contract call or value transfer
		gasUsed, logs, err = e.callContract(tx, stateDB, from)
	}

	// Determine transaction status
	var status uint64 = 1 // Success
	if err != nil {
		status = 0 // Failure
		e.logger.Warnf("Transaction execution failed: %v", err)
	}

	// Refund unused gas
	refund := new(big.Int).Mul(effectiveGasPrice, big.NewInt(int64(tx.GasLimit-gasUsed)))
	stateDB.AddBalance(from, refund)

	// Pay gas fee to block proposer (coinbase)
	gasPayment := new(big.Int).Mul(effectiveGasPrice, big.NewInt(int64(gasUsed)))
	stateDB.AddBalance(block.Header.Coinbase, gasPayment)

	// Create transaction receipt
	receipt := &TransactionReceipt{
		TxHash:            tx.Hash(),
		TxIndex:           txIndex,
		BlockHash:         block.Hash(),
		BlockNumber:       block.Header.Number,
		From:              from,
		To:                tx.To,
		CumulativeGasUsed: cumulativeGasUsed + gasUsed,
		GasUsed:           gasUsed,
		ContractAddress:   contractAddress,
		Logs:              logs,
		LogsBloom:         e.createLogsBloom(logs),
		Status:            status,
		EffectiveGasPrice: effectiveGasPrice,
	}

	return receipt, nil
}

// createContract handles contract creation transactions (now delegated to simple_vm.go)
func (e *Executor) createContract(tx *txpool.Transaction, stateDB StateDB, from txpool.Address) (*txpool.Address, uint64, []*Log, error) {
	return e.CreateContractWithEVM(tx, stateDB, from)
}

// callContract handles contract calls and value transfers (now delegated to simple_vm.go)
func (e *Executor) callContract(tx *txpool.Transaction, stateDB StateDB, from txpool.Address) (uint64, []*Log, error) {
	return e.CallContractWithEVM(tx, stateDB, from)
}

// formatWei formats wei to ETH string
func formatWei(wei *big.Int) string {
	if wei == nil {
		return "0"
	}
	eth := new(big.Float).SetInt(wei)
	eth.Quo(eth, big.NewFloat(1e18))
	return fmt.Sprintf("%.6f", eth)
}

// generateContractAddress generates a contract address from sender and nonce
func (e *Executor) generateContractAddress(from txpool.Address, nonce uint64) txpool.Address {
	// Simplified contract address generation
	// In Ethereum, this uses RLP encoding of [sender, nonce]
	var addr txpool.Address
	copy(addr[:], from[:])
	addr[19] = byte(nonce) // Simple nonce mixing
	return addr
}

// createLogsBloom creates a bloom filter for the given logs
func (e *Executor) createLogsBloom(logs []*Log) []byte {
	bloom := make([]byte, 256)

	for _, log := range logs {
		// Add address to bloom
		addToBloom(bloom, log.Address[:])

		// Add topics to bloom
		for _, topic := range log.Topics {
			addToBloom(bloom, topic[:])
		}
	}

	return bloom
}

// createFailedReceipt creates a receipt for a failed transaction
func (e *Executor) createFailedReceipt(tx *txpool.Transaction, block *EVMBlock,
	txIndex uint64, cumulativeGasUsed uint64, execErr error) *TransactionReceipt {

	// For failed transactions, we still charge gas
	gasUsed := uint64(21000) // Minimum gas

	return &TransactionReceipt{
		TxHash:            tx.Hash(),
		TxIndex:           txIndex,
		BlockHash:         block.Hash(),
		BlockNumber:       block.Header.Number,
		From:              txpool.Address{}, // Will be filled by caller
		To:                tx.To,
		CumulativeGasUsed: cumulativeGasUsed + gasUsed,
		GasUsed:           gasUsed,
		ContractAddress:   nil,
		Logs:              []*Log{},
		LogsBloom:         make([]byte, 256),
		Status:            0, // Failed
		EffectiveGasPrice: tx.GasPrice,
	}
}

// EstimateGas estimates the gas required for a transaction
func (e *Executor) EstimateGas(tx *txpool.Transaction, stateDB StateDB) (uint64, error) {
	// Create a copy of the state for estimation
	stateCopy := stateDB.Copy()

	// Get sender
	from, err := e.getSender(tx)
	if err != nil {
		return 0, err
	}

	// Validate transaction
	if err := e.validateTransaction(tx, stateCopy, from); err != nil {
		return 0, err
	}

	// Estimate based on transaction type
	var gasEstimate uint64 = 21000 // Base transaction cost

	if tx.To == nil {
		// Contract creation
		gasEstimate += 32000 + uint64(len(tx.Data)*200)
	} else {
		// Contract call or transfer
		if len(tx.Data) > 0 {
			gasEstimate += uint64(len(tx.Data) * 16)
			if stateCopy.GetCodeSize(*tx.To) > 0 {
				gasEstimate += 700 // Contract call overhead
			}
		}
	}

	return gasEstimate, nil
}

// ValidateTransactionList validates a list of transactions for block inclusion
func (e *Executor) ValidateTransactionList(transactions []*txpool.Transaction, stateDB StateDB) error {
	// Check total gas limit
	var totalGas uint64
	for _, tx := range transactions {
		totalGas += tx.GasLimit
	}

	if totalGas > e.config.GasLimit {
		return fmt.Errorf("transaction list exceeds block gas limit: %d > %d", totalGas, e.config.GasLimit)
	}

	// Validate individual transactions
	stateCopy := stateDB.Copy()
	for i, tx := range transactions {
		from, err := e.getSender(tx)
		if err != nil {
			return fmt.Errorf("transaction %d: failed to get sender: %w", i, err)
		}

		if err := e.validateTransaction(tx, stateCopy, from); err != nil {
			return fmt.Errorf("transaction %d: validation failed: %w", i, err)
		}

		// Apply transaction to state copy for next validation
		stateCopy.SetNonce(from, stateCopy.GetNonce(from)+1)
		cost := new(big.Int).Mul(tx.GasPrice, big.NewInt(int64(tx.GasLimit)))
		cost = cost.Add(cost, tx.Value)
		stateCopy.SubBalance(from, cost)
	}

	return nil
}
