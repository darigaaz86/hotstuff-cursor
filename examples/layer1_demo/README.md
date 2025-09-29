# 🚀 HotStuff Layer 1 Blockchain Demo

This demo shows how to run HotStuff as a complete Layer 1 EVM blockchain and deploy/interact with smart contracts.

## 🎯 What This Demo Covers

1. **Starting the Layer 1 Blockchain** with RPC API
2. **Deploying an ERC-20 Token Contract**
3. **Transferring Tokens** via contract function calls
4. **Querying Contract State** through RPC

## 📋 Prerequisites

- Go 1.19+ installed
- HotStuff blockchain compiled (`make`)
- Basic understanding of Ethereum/smart contracts

## 🏃‍♂️ Quick Start

```bash
# 1. Clone and build (if not done already)
cd /path/to/hotstuff
make

# 2. Run the demo
go run examples/layer1_demo/main.go
```

## 🔧 Manual Step-by-Step

### Step 1: Start the Blockchain

```bash
# Start HotStuff with RPC API enabled (CLEAN - no automatic client logs)
./hotstuff run --rpc --rpc-addr 127.0.0.1:8545 --duration 300s --replicas 4 --clients 0
```

**💡 Important**: Use `--clients 0` to avoid noisy automatic command logs like:

```
INFO cli1 client/client.go:255 25674 commands sent so far
```

This starts:

- ✅ 4 HotStuff consensus replicas  
- ✅ **No automatic clients** (clean operation)
- ✅ JSON-RPC server on port 8545
- ✅ EVM execution engine
- ✅ Merkle Patricia Trie state

**Alternative with clients** (if you need automatic transaction generation):

```bash
# With automatic clients (noisy logs)
./hotstuff run --rpc --rpc-addr 127.0.0.1:8545 --duration 300s --replicas 4 --clients 2
```

### Step 2: Test RPC Connection

First, verify the RPC server is working:

```bash
# Test chain ID
curl -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}'
# Expected: {"jsonrpc":"2.0","result":"0x539","id":1}

# Test block number
curl -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":2}'
# Expected: {"jsonrpc":"2.0","result":"0x0","id":2}
```

### Step 3: Deploy Smart Contract

**🎯 IMPORTANT:** For working contract deployment, use the interactive demo. Manual curl is limited due to transaction processing.

**Option A: Use the Interactive Demo (✅ RECOMMENDED)**

```bash
# Run the complete demo with automatic block processing
go run examples/layer1_demo/main.go
# Choose option 1 (Full Demo) or option 2 (Contract Only)
```

✅ **What the demo provides:**
- Automatic transaction execution and block creation
- Real contract addresses and state verification
- Proper EVM execution with storage updates
- Working RPC integration testing

**Option B: Manual curl (⚠️ LIMITED - transactions stay in mempool)**

```bash
# This submits to transaction pool but doesn't auto-process into blocks
curl -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_sendTransaction", 
    "params": [{
      "from": "0x1234567890123456789012345678901234567890",
      "data": "0x6064600055606460015560006000f3",
      "gas": "0x7a120",
      "gasPrice": "0x3b9aca00",
      "value": "0x0"
    }],
    "id": 1
  }'
# Returns transaction hash, but contract won't be deployed until block processing
```

**⚠️ Why manual curl has limitations:**
- HotStuff RPC integration doesn't auto-create blocks for incoming transactions
- Transactions go to mempool but need block production to execute
- Contract deployment requires transaction execution to generate addresses

**Option C: Using the Layer 1 Demo Contract**

The `examples/layer1_demo/main.go` includes a working ERC-20 token contract with this bytecode:

```solidity
// Simple token contract that:
// 1. Stores total supply (100 tokens) at storage slot 0
// 2. Stores deployer balance (100 tokens) at storage slot 1
contract SimpleToken {
    mapping(address => uint256) public balances;
    uint256 public totalSupply = 100;
    
    constructor() {
        balances[msg.sender] = totalSupply;
    }
}
```

Bytecode (hex):

```
0x6064600055606460015560006000f3
```

**Bytecode breakdown:**

- `60 64` - PUSH1 0x64 (100 in decimal)
- `60 00` - PUSH1 0x00 (storage slot 0)
- `55` - SSTORE (store total supply = 100)
- `60 64` - PUSH1 0x64 (100 tokens)
- `60 01` - PUSH1 0x01 (storage slot 1)
- `55` - SSTORE (store deployer balance = 100)
- `60 00` - PUSH1 0x00 (return offset)
- `60 00` - PUSH1 0x00 (return size)
- `f3` - RETURN (end constructor)

### Step 4: Interact with Contract

**✅ Interactive Demo Output Example:**

When you run `go run examples/layer1_demo/main.go`, you'll see output like:
```
✅ Token contract deployed at: 0x59221ccb2e2c66164d141ad9d6a6171bbb157900
📊 Contract Details:
   • Name: HotStuff Token (HST)
   • Total Supply: 1,000,000 tokens
   • Deployer token balance: 100 HST
```

**Test RPC with the deployed contract:**

```bash
# Check chain ID
curl -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}'
# Expected: {"jsonrpc":"2.0","result":"0x539","id":1}

# Check current block number  
curl -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
# Expected: {"jsonrpc":"2.0","result":"0x0","id":1}

# Check contract storage (use actual address from demo output)
curl -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_getStorageAt",
    "params": [
      "0x59221ccb2e2c66164d141ad9d6a6171bbb157900",
      "0x0000000000000000000000000000000000000000000000000000000000000000",
      "latest"
    ],
    "id": 1
  }'
# Expected: Storage value in hex format
```

**💡 Pro tip:** The interactive demo shows exactly which contract address to use for RPC calls!

## ✅ **Clean Operation Tips**

### Recommended Commands

**For clean development (no client noise):**

```bash
# Clean consensus logs only
./hotstuff run --rpc --rpc-addr 127.0.0.1:8545 --duration 300s --replicas 4 --clients 0
```

**For testing with automatic transactions:**

```bash
# With transaction generation (noisy)
./hotstuff run --rpc --rpc-addr 127.0.0.1:8545 --duration 300s --replicas 4 --clients 2
```

### What You'll See

**Clean operation (`--clients 0`):**

- ✅ Consensus logs: `Creating replicas...`, `Starting replicas...`
- ✅ RPC server logs: `Starting JSON-RPC server on 127.0.0.1:8545`
- ✅ **No client spam**: No "commands sent so far" messages

**With clients (`--clients 2`):**

- ⚠️ Noisy logs: `INFO cli1 client/client.go:255 25674 commands sent so far`
- ⚠️ Continuous transaction generation

## 🎮 Interactive Demo Features

The demo script provides:

- 🏗️ Automatic blockchain startup
- 💰 Pre-funded test accounts
- 📄 ERC-20 token contract deployment
- 🔄 Token transfer transactions
- 📊 Balance queries and verification
- 🌐 MetaMask connection instructions

## 🔗 Next Steps

1. **Connect MetaMask**: Use RPC URL `http://127.0.0.1:8545`, Chain ID `1337`
2. **Deploy Custom Contracts**: Use Remix IDE with the RPC endpoint
3. **Build DApps**: Connect with Web3.js or Ethers.js
4. **Scale Up**: Add more replicas for higher throughput

## 🛠️ Troubleshooting

### RPC Server Not Starting

```bash
# Check if RPC flags are properly set
./hotstuff run --rpc --rpc-addr 127.0.0.1:8545 --replicas 4 --clients 0

# Verify configuration
curl -X POST http://127.0.0.1:8545 -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}'
```

### Too Many Logs

```bash
# Use --clients 0 to eliminate automatic transaction logs
./hotstuff run --rpc --clients 0 --replicas 4

# Or filter logs (keeping only important ones)
./hotstuff run --rpc --clients 0 --replicas 4 2>&1 | grep -E "(INFO|ERROR|Starting|RPC)"
```

### Port Already in Use

```bash
# Use a different port
./hotstuff run --rpc --rpc-addr 127.0.0.1:8546 --clients 0 --replicas 4

# Or kill existing processes
pkill hotstuff
```

## 📚 Learn More

- Check `examples/layer1_demo/main.go` for full source code
- See RPC API documentation in `rpc/` directory
- Explore EVM implementation in `evm/` directory
