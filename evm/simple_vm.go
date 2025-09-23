package evm

import (
	"fmt"
	"math/big"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/txpool"
)

// SimpleEVM implements a basic EVM for contract execution
type SimpleEVM struct {
	stateDB StateDB
	logger  logging.Logger
}

// NewSimpleEVM creates a new simple EVM instance
func NewSimpleEVM(stateDB StateDB) *SimpleEVM {
	return &SimpleEVM{
		stateDB: stateDB,
		logger:  logging.New("simple-evm"),
	}
}

// ExecuteContract executes a contract with the given parameters
func (evm *SimpleEVM) ExecuteContract(
	caller txpool.Address,
	contract txpool.Address,
	input []byte,
	value *big.Int,
	gasLimit uint64,
	isCreate bool,
) (ret []byte, gasUsed uint64, err error) {
	
	// Basic gas accounting
	gasUsed = 21000 // Base transaction cost
	
	if isCreate {
		// Contract creation
		gasUsed += 32000 // Contract creation cost
		
		// Get the bytecode to deploy
		code := input
		if len(code) > 0 {
			gasUsed += uint64(len(code)) * 200 // Code storage cost
			
		// Simple bytecode execution for constructor
		_, constructorGas, err := evm.executeSimpleBytecode(contract, code, input, gasLimit-gasUsed)
		gasUsed += constructorGas
		
		if err != nil {
			return nil, gasUsed, err
		}
			
			// Store the runtime code (simplified - just store the constructor bytecode)
			evm.stateDB.SetCode(contract, code)
			evm.logger.Infof("Contract created at %s with %d bytes of code", contract.String(), len(code))
		}
		
		// Transfer value if any
		if value.Sign() > 0 {
			evm.stateDB.SubBalance(caller, value)
			evm.stateDB.AddBalance(contract, value)
		}
		
		return ret, gasUsed, nil
	} else {
		// Contract call
		code := evm.stateDB.GetCode(contract)
		if len(code) == 0 {
			// No code, just transfer value
			if value.Sign() > 0 {
				evm.stateDB.SubBalance(caller, value)
				evm.stateDB.AddBalance(contract, value)
			}
			return nil, gasUsed, nil
		}
		
		// Execute contract code
		ret, execGas, err := evm.executeSimpleBytecode(contract, code, input, gasLimit-gasUsed)
		gasUsed += execGas
		
		// Transfer value if any
		if value.Sign() > 0 {
			evm.stateDB.SubBalance(caller, value)
			evm.stateDB.AddBalance(contract, value)
		}
		
		return ret, gasUsed, err
	}
}

// executeSimpleBytecode executes a simplified version of EVM bytecode
func (evm *SimpleEVM) executeSimpleBytecode(
	contractAddr txpool.Address,
	code []byte,
	input []byte,
	gasLimit uint64,
) ([]byte, uint64, error) {
	
	gasUsed := uint64(0)
	stack := make([]*big.Int, 0, 1024)
	memory := make([]byte, 0)
	pc := 0
	
	// Simple execution loop
	for pc < len(code) && gasUsed < gasLimit {
		opcode := code[pc]
		gasUsed += 3 // Basic gas cost per operation
		
		switch opcode {
		case 0x00: // STOP
			return nil, gasUsed, nil
			
		case 0x01: // ADD
			if len(stack) < 2 {
				return nil, gasUsed, fmt.Errorf("stack underflow")
			}
			a := stack[len(stack)-1]
			b := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			result := new(big.Int).Add(a, b)
			stack = append(stack, result)
			
		case 0x02: // MUL
			if len(stack) < 2 {
				return nil, gasUsed, fmt.Errorf("stack underflow")
			}
			a := stack[len(stack)-1]
			b := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			result := new(big.Int).Mul(a, b)
			stack = append(stack, result)
			
		case 0x03: // SUB
			if len(stack) < 2 {
				return nil, gasUsed, fmt.Errorf("stack underflow")
			}
			a := stack[len(stack)-1]
			b := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			result := new(big.Int).Sub(a, b)
			stack = append(stack, result)
			
		case 0x52: // MSTORE
			if len(stack) < 2 {
				return nil, gasUsed, fmt.Errorf("stack underflow")
			}
			offset := stack[len(stack)-1].Uint64()
			value := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			
			// Expand memory if needed
			neededSize := int(offset + 32)
			if len(memory) < neededSize {
				newMemory := make([]byte, neededSize)
				copy(newMemory, memory)
				memory = newMemory
			}
			
			// Store 32-byte value in memory
			valueBytes := value.Bytes()
			copy(memory[offset:offset+32], leftPad32(valueBytes))
			
		case 0x54: // SLOAD
			if len(stack) < 1 {
				return nil, gasUsed, fmt.Errorf("stack underflow")
			}
			key := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			
			// Load from state storage
			var keyHash [32]byte
			copy(keyHash[:], leftPad32(key.Bytes()))
			value := evm.stateDB.GetState(contractAddr, keyHash)
			
			result := new(big.Int).SetBytes(value[:])
			stack = append(stack, result)
			gasUsed += 800 // SLOAD gas cost
			
		case 0x55: // SSTORE
			if len(stack) < 2 {
				return nil, gasUsed, fmt.Errorf("stack underflow")
			}
			key := stack[len(stack)-1]
			value := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			
			// Store to state storage
			var keyHash [32]byte
			copy(keyHash[:], leftPad32(key.Bytes()))
			var valueHash [32]byte
			copy(valueHash[:], leftPad32(value.Bytes()))
			
			evm.stateDB.SetState(contractAddr, keyHash, valueHash)
			gasUsed += 20000 // SSTORE gas cost
			
		case 0x60: // PUSH1
			if pc+1 >= len(code) {
				return nil, gasUsed, fmt.Errorf("incomplete PUSH1")
			}
			value := new(big.Int).SetUint64(uint64(code[pc+1]))
			stack = append(stack, value)
			pc++ // Skip the pushed byte
			
		case 0x61: // PUSH2
			if pc+2 >= len(code) {
				return nil, gasUsed, fmt.Errorf("incomplete PUSH2")
			}
			value := new(big.Int).SetBytes(code[pc+1:pc+3])
			stack = append(stack, value)
			pc += 2 // Skip the pushed bytes
			
		case 0x80: // DUP1
			if len(stack) < 1 {
				return nil, gasUsed, fmt.Errorf("stack underflow")
			}
			top := stack[len(stack)-1]
			stack = append(stack, new(big.Int).Set(top))
			
		case 0xf3: // RETURN
			if len(stack) < 2 {
				return nil, gasUsed, fmt.Errorf("stack underflow")
			}
			offset := stack[len(stack)-1].Uint64()
			length := stack[len(stack)-2].Uint64()
			
			if offset+length > uint64(len(memory)) {
				return nil, gasUsed, fmt.Errorf("memory out of bounds")
			}
			
			return memory[offset:offset+length], gasUsed, nil
			
		case 0xfd: // REVERT
			if len(stack) < 2 {
				return nil, gasUsed, fmt.Errorf("stack underflow")
			}
			offset := stack[len(stack)-1].Uint64()
			length := stack[len(stack)-2].Uint64()
			
			if offset+length > uint64(len(memory)) {
				return nil, gasUsed, fmt.Errorf("memory out of bounds")
			}
			
			return memory[offset:offset+length], gasUsed, fmt.Errorf("execution reverted")
			
		default:
			// Unsupported opcode, treat as NOP for now
			evm.logger.Debugf("Unsupported opcode 0x%02x at pc=%d", opcode, pc)
		}
		
		pc++
		
		// Stack limit check
		if len(stack) > 1024 {
			return nil, gasUsed, fmt.Errorf("stack overflow")
		}
	}
	
	// If we reach here without RETURN, return empty
	return nil, gasUsed, nil
}

// leftPad32 pads bytes to 32 bytes on the left
func leftPad32(data []byte) []byte {
	if len(data) >= 32 {
		return data[len(data)-32:]
	}
	result := make([]byte, 32)
	copy(result[32-len(data):], data)
	return result
}

// Enhanced executor methods using SimpleEVM

// CreateContractWithEVM creates a contract using the EVM
func (e *Executor) CreateContractWithEVM(tx *txpool.Transaction, stateDB StateDB, from txpool.Address) (*txpool.Address, uint64, []*Log, error) {
	// Generate contract address
	nonce := stateDB.GetNonce(from) - 1 // We already incremented it
	contractAddr := e.generateContractAddress(from, nonce)
	
	// Create the contract account
	stateDB.CreateAccount(contractAddr)
	
	// Create EVM instance
	evm := NewSimpleEVM(stateDB)
	
	// Execute contract creation
	ret, gasUsed, err := evm.ExecuteContract(
		from,
		contractAddr,
		tx.Data,
		tx.Value,
		tx.GasLimit,
		true, // isCreate
	)
	
	if err != nil {
		e.logger.Errorf("Contract creation failed: %v", err)
		return nil, gasUsed, nil, err
	}
	
	// Create event logs
	logs := []*Log{
		{
			Address:     contractAddr,
			Topics:      []hotstuff.Hash{},
			Data:        []byte(fmt.Sprintf("EVM Contract created: %d bytes code, %d bytes constructor return", len(tx.Data), len(ret))),
			BlockNumber: tx.Value,
			TxHash:      tx.Hash(),
			TxIndex:     0,
			BlockHash:   hotstuff.Hash{},
			LogIndex:    0,
			Removed:     false,
		},
	}
	
	e.logger.Infof("EVM Contract created at %s, gas used: %d", contractAddr.String(), gasUsed)
	return &contractAddr, gasUsed, logs, nil
}

// CallContractWithEVM calls a contract using the EVM
func (e *Executor) CallContractWithEVM(tx *txpool.Transaction, stateDB StateDB, from txpool.Address) (uint64, []*Log, error) {
	// Create EVM instance
	evm := NewSimpleEVM(stateDB)
	
	// Execute contract call
	ret, gasUsed, err := evm.ExecuteContract(
		from,
		*tx.To,
		tx.Data,
		tx.Value,
		tx.GasLimit,
		false, // isCreate
	)
	
	if err != nil {
		e.logger.Errorf("Contract call failed: %v", err)
	} else {
		e.logger.Infof("EVM Contract call successful, gas used: %d, return: %d bytes", gasUsed, len(ret))
	}
	
	// Create event logs
	var logs []*Log
	if tx.Value.Sign() > 0 {
		logs = append(logs, &Log{
			Address:     *tx.To,
			Topics:      []hotstuff.Hash{},
			Data:        []byte(fmt.Sprintf("EVM Value transfer: %s ETH", formatWei(tx.Value))),
			BlockNumber: tx.Value,
			TxHash:      tx.Hash(),
			TxIndex:     0,
			BlockHash:   hotstuff.Hash{},
			LogIndex:    0,
			Removed:     false,
		})
	}
	
	if len(tx.Data) > 0 && stateDB.GetCodeSize(*tx.To) > 0 {
		logs = append(logs, &Log{
			Address:     *tx.To,
			Topics:      []hotstuff.Hash{},
			Data:        []byte(fmt.Sprintf("EVM Call: %d bytes input, %d bytes output, status: %v", len(tx.Data), len(ret), err == nil)),
			BlockNumber: tx.Value,
			TxHash:      tx.Hash(),
			TxIndex:     0,
			BlockHash:   hotstuff.Hash{},
			LogIndex:    0,
			Removed:     false,
		})
	}
	
	return gasUsed, logs, err
}
