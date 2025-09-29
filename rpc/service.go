package rpc

import (
	"fmt"
	"math/big"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/evm"
	"github.com/relab/hotstuff/txpool"
)

// Service defines the interface for blockchain operations
type Service interface {
	// Block operations
	GetBlockByNumber(number *big.Int, includeTxs bool) (*evm.EVMBlock, error)
	GetBlockByHash(hash hotstuff.Hash, includeTxs bool) (*evm.EVMBlock, error)
	GetLatestBlock() (*evm.EVMBlock, error)
	GetLatestBlockNumber() (*big.Int, error)

	// Transaction operations
	GetTransactionByHash(hash hotstuff.Hash) (*txpool.Transaction, *evm.EVMBlock, uint64, error)
	GetTransactionReceipt(hash hotstuff.Hash) (*evm.TransactionReceipt, *evm.EVMBlock, error)
	SendTransaction(tx *txpool.Transaction) (hotstuff.Hash, error)
	SendRawTransaction(data []byte) (hotstuff.Hash, error)

	// Account operations
	GetBalance(address txpool.Address, blockNumber *big.Int) (*big.Int, error)
	GetTransactionCount(address txpool.Address, blockNumber *big.Int) (uint64, error)
	GetCode(address txpool.Address, blockNumber *big.Int) ([]byte, error)
	GetStorageAt(address txpool.Address, position hotstuff.Hash, blockNumber *big.Int) (hotstuff.Hash, error)

	// Call operations
	Call(args CallArgs, blockNumber *big.Int) ([]byte, error)
	EstimateGas(args CallArgs) (uint64, error)

	// Network operations
	ChainID() *big.Int
	GasPrice() *big.Int

	// Utility operations
	GetLogs(filter LogFilter) ([]evm.Log, error)
}

// LogFilter represents a filter for eth_getLogs
type LogFilter struct {
	FromBlock *big.Int          `json:"fromBlock"`
	ToBlock   *big.Int          `json:"toBlock"`
	Address   []txpool.Address  `json:"address"`
	Topics    [][]hotstuff.Hash `json:"topics"`
}

// ServiceImpl implements the Service interface
type ServiceImpl struct {
	blockchain   BlockchainService
	executor     *evm.Executor
	txpool       TxPoolService
	stateService StateService
	chainID      *big.Int
	gasPrice     *big.Int
}

// BlockchainService defines blockchain operations
type BlockchainService interface {
	GetBlockByNumber(number *big.Int) (*evm.EVMBlock, error)
	GetBlockByHash(hash hotstuff.Hash) (*evm.EVMBlock, error)
	GetLatestBlock() (*evm.EVMBlock, error)
	GetLatestBlockNumber() (*big.Int, error)
	GetTransactionInBlock(blockHash hotstuff.Hash, txHash hotstuff.Hash) (*txpool.Transaction, uint64, error)
	GetReceiptInBlock(blockHash hotstuff.Hash, txHash hotstuff.Hash) (*evm.TransactionReceipt, error)
}

// TxPoolService defines transaction pool operations
type TxPoolService interface {
	AddTransaction(tx *txpool.Transaction) error
	GetTransaction(hash hotstuff.Hash) (*txpool.Transaction, error)
	GetPendingTransactions() []*txpool.Transaction
}

// StateService defines state operations
type StateService interface {
	GetStateDB(blockNumber *big.Int) (evm.StateDB, error)
	GetLatestStateDB() evm.StateDB
}

// NewService creates a new RPC service
func NewService(
	blockchain BlockchainService,
	executor *evm.Executor,
	txpool TxPoolService,
	stateService StateService,
	chainID *big.Int,
) *ServiceImpl {
	return &ServiceImpl{
		blockchain:   blockchain,
		executor:     executor,
		txpool:       txpool,
		stateService: stateService,
		chainID:      chainID,
		gasPrice:     big.NewInt(1000000000), // 1 Gwei default
	}
}

// Block operations

func (s *ServiceImpl) GetBlockByNumber(number *big.Int, includeTxs bool) (*evm.EVMBlock, error) {
	return s.blockchain.GetBlockByNumber(number)
}

func (s *ServiceImpl) GetBlockByHash(hash hotstuff.Hash, includeTxs bool) (*evm.EVMBlock, error) {
	return s.blockchain.GetBlockByHash(hash)
}

func (s *ServiceImpl) GetLatestBlock() (*evm.EVMBlock, error) {
	return s.blockchain.GetLatestBlock()
}

func (s *ServiceImpl) GetLatestBlockNumber() (*big.Int, error) {
	return s.blockchain.GetLatestBlockNumber()
}

// Transaction operations

func (s *ServiceImpl) GetTransactionByHash(hash hotstuff.Hash) (*txpool.Transaction, *evm.EVMBlock, uint64, error) {
	// First try to get from txpool (pending transactions)
	if tx, err := s.txpool.GetTransaction(hash); err == nil && tx != nil {
		return tx, nil, 0, nil
	}

	// Then search in blocks
	// Search in the blockchain (simplified for demo)
	// latestBlock, err := s.blockchain.GetLatestBlock()
	// if err != nil {
	//	return nil, nil, 0, err
	// }

	// For demo purposes, transaction search is disabled
	// In a full implementation, search through blockchain for the transaction

	return nil, nil, 0, nil
}

func (s *ServiceImpl) GetTransactionReceipt(hash hotstuff.Hash) (*evm.TransactionReceipt, *evm.EVMBlock, error) {
	// Find transaction first
	_, block, _, err := s.GetTransactionByHash(hash)
	if err != nil || block == nil {
		return nil, nil, err
	}

	// For demo purposes, receipt lookup is simplified
	// In a full implementation, get receipt from block storage
	// receipt, err := s.blockchain.GetReceiptInBlock(block.Hash(), hash)
	// if err != nil {
	//	return nil, nil, err
	// }

	return nil, nil, fmt.Errorf("receipt lookup not implemented in demo")
}

func (s *ServiceImpl) SendTransaction(tx *txpool.Transaction) (hotstuff.Hash, error) {
	if err := s.txpool.AddTransaction(tx); err != nil {
		return hotstuff.Hash{}, err
	}
	// Convert txpool.Hash to hotstuff.Hash
	txHash := tx.Hash()
	var hotstuffHash hotstuff.Hash
	copy(hotstuffHash[:], txHash[:])
	return hotstuffHash, nil
}

func (s *ServiceImpl) SendRawTransaction(data []byte) (hotstuff.Hash, error) {
	// Decode RLP transaction (simplified)
	tx, err := DecodeRLPTransaction(data)
	if err != nil {
		return hotstuff.Hash{}, err
	}

	return s.SendTransaction(tx)
}

// Account operations

func (s *ServiceImpl) GetBalance(address txpool.Address, blockNumber *big.Int) (*big.Int, error) {
	stateDB, err := s.getStateDB(blockNumber)
	if err != nil {
		return nil, err
	}

	return stateDB.GetBalance(address), nil
}

func (s *ServiceImpl) GetTransactionCount(address txpool.Address, blockNumber *big.Int) (uint64, error) {
	stateDB, err := s.getStateDB(blockNumber)
	if err != nil {
		return 0, err
	}

	return stateDB.GetNonce(address), nil
}

func (s *ServiceImpl) GetCode(address txpool.Address, blockNumber *big.Int) ([]byte, error) {
	stateDB, err := s.getStateDB(blockNumber)
	if err != nil {
		return nil, err
	}

	return stateDB.GetCode(address), nil
}

func (s *ServiceImpl) GetStorageAt(address txpool.Address, position hotstuff.Hash, blockNumber *big.Int) (hotstuff.Hash, error) {
	stateDB, err := s.getStateDB(blockNumber)
	if err != nil {
		return hotstuff.Hash{}, err
	}

	return stateDB.GetState(address, position), nil
}

// Call operations

func (s *ServiceImpl) Call(args CallArgs, blockNumber *big.Int) ([]byte, error) {
	stateDB, err := s.getStateDB(blockNumber)
	if err != nil {
		return nil, err
	}

	// Convert call args to transaction
	tx, err := args.ToTxpoolTransaction()
	if err != nil {
		return nil, err
	}

	// Set default from address if not provided
	if args.From == nil {
		var defaultAddr txpool.Address
		tx.To = &defaultAddr
	} else {
		from, err := args.From.ToTxpoolAddress()
		if err != nil {
			return nil, err
		}
		// For calls, we use the from address as the caller
		_ = from
	}

	// For demo purposes, create a simple execution context
	// latestBlock, err := s.blockchain.GetLatestBlock()
	// if err != nil {
	//	return nil, err
	// }

	// Execute the call (read-only)
	if tx.To == nil {
		// Contract creation call
		evm := evm.NewSimpleEVM(stateDB)
		var contractAddr txpool.Address // Generated address
		ret, _, err := evm.ExecuteContract(
			txpool.Address{}, // Zero address for calls
			contractAddr,
			tx.Data,
			tx.Value,
			tx.GasLimit,
			true, // isCreate
		)
		return ret, err
	} else {
		// Contract call
		evm := evm.NewSimpleEVM(stateDB)
		ret, _, err := evm.ExecuteContract(
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

func (s *ServiceImpl) EstimateGas(args CallArgs) (uint64, error) {
	// Simplified gas estimation
	if args.Gas != nil {
		gas, err := args.Gas.ToUint64()
		if err != nil {
			return 0, err
		}
		return gas, nil
	}

	// Default estimation based on operation type
	if args.To == nil {
		// Contract creation
		return 200000, nil
	} else {
		// Contract call or transfer
		if args.Data != nil && len(*args.Data) > 2 {
			// Contract call
			return 100000, nil
		} else {
			// Simple transfer
			return 21000, nil
		}
	}
}

// Network operations

func (s *ServiceImpl) ChainID() *big.Int {
	return new(big.Int).Set(s.chainID)
}

func (s *ServiceImpl) GasPrice() *big.Int {
	return new(big.Int).Set(s.gasPrice)
}

// Utility operations

func (s *ServiceImpl) GetLogs(filter LogFilter) ([]evm.Log, error) {
	// Simplified log filtering - in production, use log index
	var logs []evm.Log

	fromBlock := filter.FromBlock
	if fromBlock == nil {
		fromBlock = big.NewInt(0)
	}

	toBlock := filter.ToBlock
	if toBlock == nil {
		latest, err := s.GetLatestBlockNumber()
		if err != nil {
			return nil, err
		}
		toBlock = latest
	}

	// Iterate through blocks and collect matching logs
	for blockNum := new(big.Int).Set(fromBlock); blockNum.Cmp(toBlock) <= 0; blockNum.Add(blockNum, big.NewInt(1)) {
		block, err := s.blockchain.GetBlockByNumber(blockNum)
		if err != nil {
			continue
		}

		// Check each transaction's logs
		for _, tx := range block.Transactions {
			// Convert tx hash for compatibility
			txHash := tx.Hash()
			var hotstuffHash hotstuff.Hash
			copy(hotstuffHash[:], txHash[:])

			// Demo: receipt lookup not implemented
			// receipt, err := s.blockchain.GetReceiptInBlock(block.Hash(), hotstuffHash)
			// if err != nil {
			//	continue
			// }

			// Skip receipt processing for demo
			// In a full implementation, process receipt logs here
		}
	}

	return logs, nil
}

// Helper methods

func (s *ServiceImpl) getStateDB(blockNumber *big.Int) (evm.StateDB, error) {
	if blockNumber == nil {
		return s.stateService.GetLatestStateDB(), nil
	}

	return s.stateService.GetStateDB(blockNumber)
}

func (s *ServiceImpl) matchesLogFilter(log *evm.Log, filter LogFilter) bool {
	// Check address filter
	if len(filter.Address) > 0 {
		found := false
		for _, addr := range filter.Address {
			if log.Address == addr {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check topics filter (simplified)
	for i, topicFilter := range filter.Topics {
		if i >= len(log.Topics) {
			break
		}

		if len(topicFilter) > 0 {
			found := false
			for _, topic := range topicFilter {
				if log.Topics[i] == topic {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	return true
}

// DecodeRLPTransaction decodes a raw transaction (simplified)
func DecodeRLPTransaction(data []byte) (*txpool.Transaction, error) {
	// This is a simplified decoder - in production, use proper RLP decoding
	if len(data) < 10 {
		return nil, fmt.Errorf("transaction data too short")
	}

	// For now, create a minimal transaction
	tx := &txpool.Transaction{
		Nonce:    0,
		GasPrice: big.NewInt(1000000000),
		GasLimit: 21000,
		Value:    big.NewInt(0),
		Data:     data[10:], // Skip some header bytes
		V:        big.NewInt(0),
		R:        big.NewInt(0),
		S:        big.NewInt(0),
		ChainID:  big.NewInt(1337),
	}

	return tx, nil
}
