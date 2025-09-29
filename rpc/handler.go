package rpc

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/logging"
)

// Handler implements the Ethereum JSON-RPC API
type Handler struct {
	service Service
	logger  logging.Logger
}

// NewHandler creates a new RPC handler
func NewHandler(service Service) *Handler {
	return &Handler{
		service: service,
		logger:  logging.New("rpc"),
	}
}

// ServeHTTP implements the http.Handler interface
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		h.writeError(w, nil, NewRPCError(MethodNotFound, "method not found", nil))
		return
	}

	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, nil, NewRPCError(ParseError, "parse error", err.Error()))
		return
	}

	result, err := h.handleRequest(&req)
	if err != nil {
		h.writeError(w, req.ID, err)
		return
	}

	response := JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Errorf("Failed to encode response: %v", err)
	}
}

// handleRequest processes a JSON-RPC request
func (h *Handler) handleRequest(req *JSONRPCRequest) (interface{}, *RPCError) {
	h.logger.Debugf("RPC call: %s", req.Method)

	switch req.Method {
	// Network identification
	case "eth_chainId":
		return h.chainID(req.Params)
	case "net_version":
		return h.netVersion(req.Params)
	case "web3_clientVersion":
		return "HotStuff-EVM/1.0.0", nil

	// Block methods
	case "eth_blockNumber":
		return h.blockNumber(req.Params)
	case "eth_getBlockByNumber":
		return h.getBlockByNumber(req.Params)
	case "eth_getBlockByHash":
		return h.getBlockByHash(req.Params)

	// Transaction methods
	case "eth_getTransactionByHash":
		return h.getTransactionByHash(req.Params)
	case "eth_getTransactionReceipt":
		return h.getTransactionReceipt(req.Params)
	case "eth_sendTransaction":
		return h.sendTransaction(req.Params)
	case "eth_sendRawTransaction":
		return h.sendRawTransaction(req.Params)

	// Account methods
	case "eth_getBalance":
		return h.getBalance(req.Params)
	case "eth_getTransactionCount":
		return h.getTransactionCount(req.Params)
	case "eth_getCode":
		return h.getCode(req.Params)
	case "eth_getStorageAt":
		return h.getStorageAt(req.Params)

	// Call methods
	case "eth_call":
		return h.call(req.Params)
	case "eth_estimateGas":
		return h.estimateGas(req.Params)

	// Gas methods
	case "eth_gasPrice":
		return h.gasPrice(req.Params)

	// Log methods
	case "eth_getLogs":
		return h.getLogs(req.Params)

	// Utility methods
	case "eth_accounts":
		return []string{}, nil // No managed accounts
	case "eth_mining":
		return false, nil // Not a PoW chain
	case "eth_syncing":
		return false, nil // Always synced for now

	default:
		return nil, NewRPCError(MethodNotFound, fmt.Sprintf("method %s not found", req.Method), nil)
	}
}

// Network identification methods

func (h *Handler) chainID(params json.RawMessage) (interface{}, *RPCError) {
	chainID := h.service.ChainID()
	return NewHexNumberFromBig(chainID), nil
}

func (h *Handler) netVersion(params json.RawMessage) (interface{}, *RPCError) {
	chainID := h.service.ChainID()
	return chainID.String(), nil
}

// Block methods

func (h *Handler) blockNumber(params json.RawMessage) (interface{}, *RPCError) {
	number, err := h.service.GetLatestBlockNumber()
	if err != nil {
		return nil, NewRPCError(InternalError, "failed to get block number", err.Error())
	}
	return NewHexNumberFromBig(number), nil
}

func (h *Handler) getBlockByNumber(params json.RawMessage) (interface{}, *RPCError) {
	var args []interface{}
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, NewRPCError(InvalidParams, "invalid parameters", err.Error())
	}

	if len(args) < 2 {
		return nil, NewRPCError(InvalidParams, "missing parameters", nil)
	}

	blockNumber, err := h.parseBlockNumber(args[0])
	if err != nil {
		return nil, NewRPCError(InvalidParams, "invalid block number", err.Error())
	}

	includeTxs, ok := args[1].(bool)
	if !ok {
		return nil, NewRPCError(InvalidParams, "invalid includeTxs parameter", nil)
	}

	block, err := h.service.GetBlockByNumber(blockNumber, includeTxs)
	if err != nil {
		return nil, NewRPCError(InternalError, "failed to get block", err.Error())
	}

	if block == nil {
		return nil, nil
	}

	return NewBlockFromEVM(block, includeTxs), nil
}

func (h *Handler) getBlockByHash(params json.RawMessage) (interface{}, *RPCError) {
	var args []interface{}
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, NewRPCError(InvalidParams, "invalid parameters", err.Error())
	}

	if len(args) < 2 {
		return nil, NewRPCError(InvalidParams, "missing parameters", nil)
	}

	hashStr, ok := args[0].(string)
	if !ok {
		return nil, NewRPCError(InvalidParams, "invalid hash parameter", nil)
	}

	hash, err := Hash(hashStr).ToHotstuffHash()
	if err != nil {
		return nil, NewRPCError(InvalidParams, "invalid hash format", err.Error())
	}

	includeTxs, ok := args[1].(bool)
	if !ok {
		return nil, NewRPCError(InvalidParams, "invalid includeTxs parameter", nil)
	}

	block, err := h.service.GetBlockByHash(hash, includeTxs)
	if err != nil {
		return nil, NewRPCError(InternalError, "failed to get block", err.Error())
	}

	if block == nil {
		return nil, nil
	}

	return NewBlockFromEVM(block, includeTxs), nil
}

// Transaction methods

func (h *Handler) getTransactionByHash(params json.RawMessage) (interface{}, *RPCError) {
	var args []string
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, NewRPCError(InvalidParams, "invalid parameters", err.Error())
	}

	if len(args) < 1 {
		return nil, NewRPCError(InvalidParams, "missing hash parameter", nil)
	}

	hash, err := Hash(args[0]).ToHotstuffHash()
	if err != nil {
		return nil, NewRPCError(InvalidParams, "invalid hash format", err.Error())
	}

	tx, block, txIndex, err := h.service.GetTransactionByHash(hash)
	if err != nil {
		return nil, NewRPCError(InternalError, "failed to get transaction", err.Error())
	}

	if tx == nil {
		return nil, nil
	}

	var blockHash *hotstuff.Hash
	var blockNumber *big.Int
	var index *uint64

	if block != nil {
		blockHash = &hotstuff.Hash{}
		*blockHash = block.Hash()
		blockNumber = block.Header.Number
		index = &txIndex
	}

	return NewTransactionFromTxpool(tx, blockHash, blockNumber, index), nil
}

func (h *Handler) getTransactionReceipt(params json.RawMessage) (interface{}, *RPCError) {
	var args []string
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, NewRPCError(InvalidParams, "invalid parameters", err.Error())
	}

	if len(args) < 1 {
		return nil, NewRPCError(InvalidParams, "missing hash parameter", nil)
	}

	hash, err := Hash(args[0]).ToHotstuffHash()
	if err != nil {
		return nil, NewRPCError(InvalidParams, "invalid hash format", err.Error())
	}

	receipt, block, err := h.service.GetTransactionReceipt(hash)
	if err != nil {
		return nil, NewRPCError(InternalError, "failed to get receipt", err.Error())
	}

	if receipt == nil || block == nil {
		return nil, nil
	}

	return NewTransactionReceiptFromEVM(receipt, block.Hash(), block.Header.Number), nil
}

func (h *Handler) sendTransaction(params json.RawMessage) (interface{}, *RPCError) {
	var args []CallArgs
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, NewRPCError(InvalidParams, "invalid parameters", err.Error())
	}

	if len(args) < 1 {
		return nil, NewRPCError(InvalidParams, "missing transaction object", nil)
	}

	tx, err := args[0].ToTxpoolTransaction()
	if err != nil {
		return nil, NewRPCError(InvalidParams, "invalid transaction", err.Error())
	}

	hash, err := h.service.SendTransaction(tx)
	if err != nil {
		return nil, NewRPCError(InternalError, "failed to send transaction", err.Error())
	}

	return NewHash(hash), nil
}

func (h *Handler) sendRawTransaction(params json.RawMessage) (interface{}, *RPCError) {
	var args []string
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, NewRPCError(InvalidParams, "invalid parameters", err.Error())
	}

	if len(args) < 1 {
		return nil, NewRPCError(InvalidParams, "missing data parameter", nil)
	}

	data, err := HexBytes(args[0]).ToBytes()
	if err != nil {
		return nil, NewRPCError(InvalidParams, "invalid hex data", err.Error())
	}

	hash, err := h.service.SendRawTransaction(data)
	if err != nil {
		return nil, NewRPCError(InternalError, "failed to send transaction", err.Error())
	}

	return NewHash(hash), nil
}

// Account methods

func (h *Handler) getBalance(params json.RawMessage) (interface{}, *RPCError) {
	var args []interface{}
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, NewRPCError(InvalidParams, "invalid parameters", err.Error())
	}

	if len(args) < 2 {
		return nil, NewRPCError(InvalidParams, "missing parameters", nil)
	}

	addressStr, ok := args[0].(string)
	if !ok {
		return nil, NewRPCError(InvalidParams, "invalid address parameter", nil)
	}

	address, err := Address(addressStr).ToTxpoolAddress()
	if err != nil {
		return nil, NewRPCError(InvalidParams, "invalid address format", err.Error())
	}

	blockNumber, err := h.parseBlockNumber(args[1])
	if err != nil {
		return nil, NewRPCError(InvalidParams, "invalid block number", err.Error())
	}

	balance, err := h.service.GetBalance(address, blockNumber)
	if err != nil {
		return nil, NewRPCError(InternalError, "failed to get balance", err.Error())
	}

	return NewHexNumberFromBig(balance), nil
}

func (h *Handler) getTransactionCount(params json.RawMessage) (interface{}, *RPCError) {
	var args []interface{}
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, NewRPCError(InvalidParams, "invalid parameters", err.Error())
	}

	if len(args) < 2 {
		return nil, NewRPCError(InvalidParams, "missing parameters", nil)
	}

	addressStr, ok := args[0].(string)
	if !ok {
		return nil, NewRPCError(InvalidParams, "invalid address parameter", nil)
	}

	address, err := Address(addressStr).ToTxpoolAddress()
	if err != nil {
		return nil, NewRPCError(InvalidParams, "invalid address format", err.Error())
	}

	blockNumber, err := h.parseBlockNumber(args[1])
	if err != nil {
		return nil, NewRPCError(InvalidParams, "invalid block number", err.Error())
	}

	count, err := h.service.GetTransactionCount(address, blockNumber)
	if err != nil {
		return nil, NewRPCError(InternalError, "failed to get transaction count", err.Error())
	}

	return NewHexNumber(count), nil
}

func (h *Handler) getCode(params json.RawMessage) (interface{}, *RPCError) {
	var args []interface{}
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, NewRPCError(InvalidParams, "invalid parameters", err.Error())
	}

	if len(args) < 2 {
		return nil, NewRPCError(InvalidParams, "missing parameters", nil)
	}

	addressStr, ok := args[0].(string)
	if !ok {
		return nil, NewRPCError(InvalidParams, "invalid address parameter", nil)
	}

	address, err := Address(addressStr).ToTxpoolAddress()
	if err != nil {
		return nil, NewRPCError(InvalidParams, "invalid address format", err.Error())
	}

	blockNumber, err := h.parseBlockNumber(args[1])
	if err != nil {
		return nil, NewRPCError(InvalidParams, "invalid block number", err.Error())
	}

	code, err := h.service.GetCode(address, blockNumber)
	if err != nil {
		return nil, NewRPCError(InternalError, "failed to get code", err.Error())
	}

	return NewHexBytes(code), nil
}

func (h *Handler) getStorageAt(params json.RawMessage) (interface{}, *RPCError) {
	var args []interface{}
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, NewRPCError(InvalidParams, "invalid parameters", err.Error())
	}

	if len(args) < 3 {
		return nil, NewRPCError(InvalidParams, "missing parameters", nil)
	}

	addressStr, ok := args[0].(string)
	if !ok {
		return nil, NewRPCError(InvalidParams, "invalid address parameter", nil)
	}

	address, err := Address(addressStr).ToTxpoolAddress()
	if err != nil {
		return nil, NewRPCError(InvalidParams, "invalid address format", err.Error())
	}

	positionStr, ok := args[1].(string)
	if !ok {
		return nil, NewRPCError(InvalidParams, "invalid position parameter", nil)
	}

	position, err := Hash(positionStr).ToHotstuffHash()
	if err != nil {
		return nil, NewRPCError(InvalidParams, "invalid position format", err.Error())
	}

	blockNumber, err := h.parseBlockNumber(args[2])
	if err != nil {
		return nil, NewRPCError(InvalidParams, "invalid block number", err.Error())
	}

	value, err := h.service.GetStorageAt(address, position, blockNumber)
	if err != nil {
		return nil, NewRPCError(InternalError, "failed to get storage", err.Error())
	}

	return NewHash(value), nil
}

// Call methods

func (h *Handler) call(params json.RawMessage) (interface{}, *RPCError) {
	var args []interface{}
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, NewRPCError(InvalidParams, "invalid parameters", err.Error())
	}

	if len(args) < 1 {
		return nil, NewRPCError(InvalidParams, "missing call object", nil)
	}

	// Parse call arguments
	callData, err := json.Marshal(args[0])
	if err != nil {
		return nil, NewRPCError(InvalidParams, "invalid call object", err.Error())
	}

	var callArgs CallArgs
	if err := json.Unmarshal(callData, &callArgs); err != nil {
		return nil, NewRPCError(InvalidParams, "invalid call object", err.Error())
	}

	var blockNumber *big.Int
	if len(args) > 1 {
		blockNumber, err = h.parseBlockNumber(args[1])
		if err != nil {
			return nil, NewRPCError(InvalidParams, "invalid block number", err.Error())
		}
	}

	result, err := h.service.Call(callArgs, blockNumber)
	if err != nil {
		return nil, NewRPCError(InternalError, "call failed", err.Error())
	}

	return NewHexBytes(result), nil
}

func (h *Handler) estimateGas(params json.RawMessage) (interface{}, *RPCError) {
	var args []CallArgs
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, NewRPCError(InvalidParams, "invalid parameters", err.Error())
	}

	if len(args) < 1 {
		return nil, NewRPCError(InvalidParams, "missing call object", nil)
	}

	gasEstimate, err := h.service.EstimateGas(args[0])
	if err != nil {
		return nil, NewRPCError(InternalError, "gas estimation failed", err.Error())
	}

	return NewHexNumber(gasEstimate), nil
}

// Gas methods

func (h *Handler) gasPrice(params json.RawMessage) (interface{}, *RPCError) {
	gasPrice := h.service.GasPrice()
	return NewHexNumberFromBig(gasPrice), nil
}

// Log methods

func (h *Handler) getLogs(params json.RawMessage) (interface{}, *RPCError) {
	var args []LogFilter
	if err := json.Unmarshal(params, &args); err != nil {
		return nil, NewRPCError(InvalidParams, "invalid parameters", err.Error())
	}

	if len(args) < 1 {
		return nil, NewRPCError(InvalidParams, "missing filter object", nil)
	}

	logs, err := h.service.GetLogs(args[0])
	if err != nil {
		return nil, NewRPCError(InternalError, "failed to get logs", err.Error())
	}

	// Convert to RPC format
	rpcLogs := make([]Log, len(logs))
	for i, log := range logs {
		topics := make([]Hash, len(log.Topics))
		for j, topic := range log.Topics {
			topics[j] = NewHash(topic)
		}

		// Convert txpool.Hash to hotstuff.Hash for API compatibility
		txHashConverted := hotstuff.Hash{}
		copy(txHashConverted[:], log.TxHash[:])

		rpcLogs[i] = Log{
			Address:          NewAddress(log.Address),
			Topics:           topics,
			Data:             NewHexBytes(log.Data),
			BlockNumber:      NewHexNumberFromBig(big.NewInt(int64(log.BlockNumber.Uint64()))),
			TransactionHash:  NewHash(txHashConverted),
			TransactionIndex: NewHexNumber(uint64(log.TxIndex)),
			BlockHash:        NewHash(log.BlockHash),
			LogIndex:         NewHexNumber(uint64(log.LogIndex)),
			Removed:          log.Removed,
		}
	}

	return rpcLogs, nil
}

// Helper methods

func (h *Handler) parseBlockNumber(param interface{}) (*big.Int, error) {
	switch v := param.(type) {
	case string:
		switch v {
		case "latest", "pending":
			return nil, nil // nil means latest
		case "earliest":
			return big.NewInt(0), nil
		default:
			if strings.HasPrefix(v, "0x") {
				num := new(big.Int)
				num.SetString(v[2:], 16)
				return num, nil
			}
			return nil, fmt.Errorf("invalid block number: %s", v)
		}
	case float64:
		return big.NewInt(int64(v)), nil
	default:
		return nil, fmt.Errorf("invalid block number type: %T", param)
	}
}

func (h *Handler) writeError(w http.ResponseWriter, id interface{}, rpcErr *RPCError) {
	response := JSONRPCResponse{
		JSONRPC: "2.0",
		Error:   rpcErr,
		ID:      id,
	}

	w.WriteHeader(http.StatusOK) // JSON-RPC errors are still HTTP 200
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Errorf("Failed to encode error response: %v", err)
	}
}
