// Package blockchain provides an implementation of the consensus.BlockChain interface using BadgerDB.
package blockchain

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"google.golang.org/protobuf/proto"
)

// Key prefixes for BadgerDB
const (
	blockPrefix  = "block:"  // block:<hash> -> serialized Block
	heightPrefix = "height:" // height:<view> -> block hash
	metaPrefix   = "meta:"   // meta:<key> -> value

	// Metadata keys
	pruneHeightKey = "meta:prune_height"
)

// badgerBlockChain is a persistent implementation of the BlockChain interface using BadgerDB.
type badgerBlockChain struct {
	configuration modules.Configuration
	consensus     modules.Consensus
	eventLoop     *eventloop.EventLoop
	logger        logging.Logger

	db *badger.DB

	mut          sync.Mutex
	pruneHeight  hotstuff.View
	pendingFetch map[hotstuff.Hash]context.CancelFunc // allows a pending fetch operation to be canceled
}

// NewBadgerBlockChain creates a new BadgerDB-backed blockchain.
// dbPath is the directory where the BadgerDB files will be stored.
func NewBadgerBlockChain(dbPath string) (modules.BlockChain, error) {
	// Configure BadgerDB options
	opts := badger.DefaultOptions(dbPath).
		WithLogger(nil) // Disable BadgerDB's own logging

	// Open BadgerDB
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open BadgerDB: %w", err)
	}

	bc := &badgerBlockChain{
		db:           db,
		pendingFetch: make(map[hotstuff.Hash]context.CancelFunc),
	}

	// Load prune height from database
	if err := bc.loadPruneHeight(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to load prune height: %w", err)
	}

	// Note: Genesis block will be stored when InitModule is called

	return bc, nil
}

func (chain *badgerBlockChain) InitModule(mods *modules.Core) {
	mods.Get(
		&chain.configuration,
		&chain.consensus,
		&chain.eventLoop,
		&chain.logger,
	)

	// Ensure genesis block is stored after logger is initialized
	if err := chain.ensureGenesis(); err != nil {
		chain.logger.Errorf("Failed to ensure genesis: %v", err)
	}
}

// Close closes the BadgerDB database.
func (chain *badgerBlockChain) Close() error {
	return chain.db.Close()
}

// Store stores a block in the blockchain
func (chain *badgerBlockChain) Store(block *hotstuff.Block) {
	chain.mut.Lock()
	defer chain.mut.Unlock()

	err := chain.db.Update(func(txn *badger.Txn) error {
		// Serialize block to protobuf
		pbBlock := hotstuffpb.BlockToProto(block)
		blockData, err := proto.Marshal(pbBlock)
		if err != nil {
			return fmt.Errorf("failed to marshal block: %w", err)
		}

		// Store block by hash
		blockKey := makeBlockKey(block.Hash())
		if err := txn.Set(blockKey, blockData); err != nil {
			return fmt.Errorf("failed to store block by hash: %w", err)
		}

		// Store block hash by height
		heightKey := makeHeightKey(block.View())
		hash := block.Hash()
		hashData := hash[:]
		if err := txn.Set(heightKey, hashData); err != nil {
			return fmt.Errorf("failed to store block by height: %w", err)
		}

		return nil
	})

	if err != nil {
		if chain.logger != nil {
			chain.logger.Errorf("Failed to store block %s: %v", block.Hash().String(), err)
		}
		return
	}

	if chain.logger != nil {
		chain.logger.Debugf("Stored block: %s at height %d", block.Hash().String(), block.View())
	}

	// cancel any pending fetch operations
	if cancel, ok := chain.pendingFetch[block.Hash()]; ok {
		cancel()
		delete(chain.pendingFetch, block.Hash())
	}
}

// LocalGet retrieves a block given its hash. It will only try the local cache.
func (chain *badgerBlockChain) LocalGet(hash hotstuff.Hash) (*hotstuff.Block, bool) {
	chain.mut.Lock()
	defer chain.mut.Unlock()

	var block *hotstuff.Block
	err := chain.db.View(func(txn *badger.Txn) error {
		blockKey := makeBlockKey(hash)
		item, err := txn.Get(blockKey)
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil // not an error, just not found
			}
			return err
		}

		return item.Value(func(val []byte) error {
			// Deserialize block from protobuf
			var pbBlock hotstuffpb.Block
			if err := proto.Unmarshal(val, &pbBlock); err != nil {
				return fmt.Errorf("failed to unmarshal block: %w", err)
			}

			block = hotstuffpb.BlockFromProto(&pbBlock)
			return nil
		})
	})

	if err != nil {
		if chain.logger != nil {
			chain.logger.Errorf("Failed to get block %s: %v", hash.String(), err)
		}
		return nil, false
	}

	return block, block != nil
}

// Get retrieves a block given its hash. Get will try to find the block locally.
// If it is not available locally, it will try to fetch the block.
func (chain *badgerBlockChain) Get(hash hotstuff.Hash) (block *hotstuff.Block, ok bool) {
	// need to declare vars early, or else we won't be able to use goto
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	chain.mut.Lock()
	block, ok = chain.localGetUnsafe(hash)
	if ok {
		goto done
	}

	ctx, cancel = context.WithTimeout(chain.eventLoop.Context(), 5*time.Second)
	chain.pendingFetch[hash] = cancel

	chain.mut.Unlock()
	if chain.logger != nil {
		chain.logger.Debugf("Attempting to fetch block: %.8s", hash)
	}
	block, ok = chain.configuration.Fetch(ctx, hash)
	chain.mut.Lock()

	delete(chain.pendingFetch, hash)
	if !ok {
		// check again in case the block arrived while we were fetching
		block, ok = chain.localGetUnsafe(hash)
		goto done
	}

	if chain.logger != nil {
		chain.logger.Debugf("Successfully fetched block: %.8s", hash)
	}

	// Store the fetched block
	chain.storeUnsafe(block)

done:
	chain.mut.Unlock()

	if !ok {
		return nil, false
	}

	return block, true
}

// Extends checks if the given block extends the branch of the target block.
func (chain *badgerBlockChain) Extends(block, target *hotstuff.Block) bool {
	current := block
	ok := true
	for ok && current.View() > target.View() {
		current, ok = chain.Get(current.Parent())
	}
	return ok && current.Hash() == target.Hash()
}

// PruneToHeight prunes blocks from the database up to the specified height.
// Returns a set of forked blocks (blocks that were on a different branch, and thus not committed).
func (chain *badgerBlockChain) PruneToHeight(height hotstuff.View) (forkedBlocks []*hotstuff.Block) {
	chain.mut.Lock()
	defer chain.mut.Unlock()

	committedHeight := chain.consensus.CommittedBlock().View()
	committedViews := make(map[hotstuff.View]bool)
	committedViews[committedHeight] = true

	// Build set of committed views by walking back from committed block
	for h := committedHeight; h >= chain.pruneHeight; {
		block, ok := chain.getByHeightUnsafe(h)
		if !ok {
			break
		}
		parent, ok := chain.localGetUnsafe(block.Parent())
		if !ok || parent.View() < chain.pruneHeight {
			break
		}
		h = parent.View()
		committedViews[h] = true
	}

	// Find forked blocks (blocks not on committed chain)
	for h := height; h > chain.pruneHeight; h-- {
		if !committedViews[h] {
			if block, ok := chain.getByHeightUnsafe(h); ok {
				if chain.logger != nil {
					chain.logger.Debugf("PruneToHeight: found forked block: %v", block)
				}
				forkedBlocks = append(forkedBlocks, block)
			}
		}
	}

	// Update prune height
	chain.pruneHeight = height
	if err := chain.savePruneHeight(); err != nil {
		if chain.logger != nil {
			chain.logger.Errorf("Failed to save prune height: %v", err)
		}
	}

	// TODO: Actually delete old blocks from database for space efficiency
	// This would require careful implementation to avoid deleting blocks that might still be needed

	return forkedBlocks
}

// Helper methods

// localGetUnsafe retrieves a block without locking (assumes lock is held)
func (chain *badgerBlockChain) localGetUnsafe(hash hotstuff.Hash) (*hotstuff.Block, bool) {
	var block *hotstuff.Block
	err := chain.db.View(func(txn *badger.Txn) error {
		blockKey := makeBlockKey(hash)
		item, err := txn.Get(blockKey)
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil
			}
			return err
		}

		return item.Value(func(val []byte) error {
			var pbBlock hotstuffpb.Block
			if err := proto.Unmarshal(val, &pbBlock); err != nil {
				return fmt.Errorf("failed to unmarshal block: %w", err)
			}

			block = hotstuffpb.BlockFromProto(&pbBlock)
			return nil
		})
	})

	if err != nil {
		if chain.logger != nil {
			chain.logger.Errorf("Failed to get block %s: %v", hash.String(), err)
		}
		return nil, false
	}

	return block, block != nil
}

// storeUnsafe stores a block without locking (assumes lock is held)
func (chain *badgerBlockChain) storeUnsafe(block *hotstuff.Block) {
	err := chain.db.Update(func(txn *badger.Txn) error {
		pbBlock := hotstuffpb.BlockToProto(block)
		blockData, err := proto.Marshal(pbBlock)
		if err != nil {
			return fmt.Errorf("failed to marshal block: %w", err)
		}

		blockKey := makeBlockKey(block.Hash())
		if err := txn.Set(blockKey, blockData); err != nil {
			return fmt.Errorf("failed to store block by hash: %w", err)
		}

		heightKey := makeHeightKey(block.View())
		hash := block.Hash()
		hashData := hash[:]
		if err := txn.Set(heightKey, hashData); err != nil {
			return fmt.Errorf("failed to store block by height: %w", err)
		}

		return nil
	})

	if err != nil {
		if chain.logger != nil {
			chain.logger.Errorf("Failed to store block %s: %v", block.Hash().String(), err)
		}
	}
}

// getByHeightUnsafe retrieves a block by height without locking (assumes lock is held)
func (chain *badgerBlockChain) getByHeightUnsafe(view hotstuff.View) (*hotstuff.Block, bool) {
	var hash hotstuff.Hash
	err := chain.db.View(func(txn *badger.Txn) error {
		heightKey := makeHeightKey(view)
		item, err := txn.Get(heightKey)
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil
			}
			return err
		}

		return item.Value(func(val []byte) error {
			if len(val) != 32 {
				return fmt.Errorf("invalid hash length: %d", len(val))
			}
			copy(hash[:], val)
			return nil
		})
	})

	if err != nil || hash == (hotstuff.Hash{}) {
		return nil, false
	}

	return chain.localGetUnsafe(hash)
}

// loadPruneHeight loads the prune height from the database
func (chain *badgerBlockChain) loadPruneHeight() error {
	return chain.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(pruneHeightKey))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				chain.pruneHeight = 0
				return nil
			}
			return err
		}

		return item.Value(func(val []byte) error {
			if len(val) != 8 {
				return fmt.Errorf("invalid prune height length: %d", len(val))
			}
			chain.pruneHeight = hotstuff.View(binary.LittleEndian.Uint64(val))
			return nil
		})
	})
}

// savePruneHeight saves the prune height to the database
func (chain *badgerBlockChain) savePruneHeight() error {
	return chain.db.Update(func(txn *badger.Txn) error {
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], uint64(chain.pruneHeight))
		return txn.Set([]byte(pruneHeightKey), buf[:])
	})
}

// ensureGenesis ensures the genesis block is stored in the database
func (chain *badgerBlockChain) ensureGenesis() error {
	genesis := hotstuff.GetGenesis()
	_, exists := chain.localGetUnsafe(genesis.Hash())
	if !exists {
		chain.storeUnsafe(genesis)
		if chain.logger != nil {
			chain.logger.Debugf("Stored genesis block: %s", genesis.Hash().String())
		}
	}
	return nil
}

// Key generation helpers

func makeBlockKey(hash hotstuff.Hash) []byte {
	key := make([]byte, len(blockPrefix)+32)
	copy(key, blockPrefix)
	copy(key[len(blockPrefix):], hash[:])
	return key
}

func makeHeightKey(view hotstuff.View) []byte {
	key := make([]byte, len(heightPrefix)+8)
	copy(key, heightPrefix)
	binary.LittleEndian.PutUint64(key[len(heightPrefix):], uint64(view))
	return key
}

var _ modules.BlockChain = (*badgerBlockChain)(nil)
