// Package simplehotstuff implements a simplified version of the three-chain HotStuff protocol with persistence.
package simplehotstuff

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
)

// PersistentSimpleHotStuff implements a simplified version of the HotStuff algorithm with persistent state.
//
// Based on the simplified algorithm described in the paper
// "Formal Verification of HotStuff" by Leander Jehl.
type PersistentSimpleHotStuff struct {
	blockChain   modules.BlockChain
	logger       logging.Logger
	synchronizer modules.Synchronizer

	// Persistent state store
	stateStore *blockchain.StateStore

	locked *hotstuff.Block
}

// NewPersistent returns a new persistent SimpleHotStuff instance.
func NewPersistent(dataDir string) (consensus.Rules, error) {
	stateStore, err := blockchain.NewStateStore(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create state store: %w", err)
	}

	return &PersistentSimpleHotStuff{
		stateStore: stateStore,
		locked:     hotstuff.GetGenesis(), // will be loaded from state store in InitModule
	}, nil
}

// InitModule initializes the module
func (hs *PersistentSimpleHotStuff) InitModule(mods *modules.Core) {
	mods.Get(&hs.blockChain, &hs.logger, &hs.synchronizer)

	// Load persistent state
	if err := hs.loadState(); err != nil {
		hs.logger.Errorf("Failed to load persistent SimpleHotStuff state: %v", err)
		// Continue with genesis if loading fails
		hs.locked = hotstuff.GetGenesis()
		hs.persistLocked()
	}
}

// loadState loads the locked block from persistent storage
func (hs *PersistentSimpleHotStuff) loadState() error {
	lockedHash, err := hs.stateStore.GetLockedBlockHash()
	if err != nil {
		return fmt.Errorf("failed to load locked block hash: %w", err)
	}

	// Get the locked block from blockchain
	if lockedBlock, ok := hs.blockChain.LocalGet(lockedHash); ok {
		hs.locked = lockedBlock
		hs.logger.Infof("Loaded locked block: %s at view %d", lockedHash.String(), lockedBlock.View())
	} else {
		hs.logger.Warnf("Locked block %s not found in blockchain, using genesis", lockedHash.String())
		hs.locked = hotstuff.GetGenesis()
	}

	return nil
}

// persistLocked saves the locked block hash to persistent storage
func (hs *PersistentSimpleHotStuff) persistLocked() {
	if err := hs.stateStore.SetLockedBlockHash(hs.locked.Hash()); err != nil {
		hs.logger.Errorf("Failed to persist locked block: %v", err)
	}
}

// VoteRule decides if the replica should vote for the given block.
func (hs *PersistentSimpleHotStuff) VoteRule(proposal hotstuff.ProposeMsg) bool {
	block := proposal.Block

	// Rule 1: can only vote in increasing rounds
	if block.View() < hs.synchronizer.View() {
		hs.logger.Info("VoteRule: block view too low")
		return false
	}

	parent, ok := hs.blockChain.Get(block.QuorumCert().BlockHash())
	if !ok {
		hs.logger.Info("VoteRule: missing parent block: ", block.QuorumCert().BlockHash())
		return false
	}

	// Rule 2: can only vote if parent's view is greater than or equal to locked block's view.
	if parent.View() < hs.locked.View() {
		hs.logger.Info("OnPropose: parent too old")
		return false
	}

	return true
}

// CommitRule decides if an ancestor of the block can be committed, and returns the ancestor, otherwise returns nil.
func (hs *PersistentSimpleHotStuff) CommitRule(block *hotstuff.Block) *hotstuff.Block {
	parent, ok := hs.blockChain.Get(block.QuorumCert().BlockHash())
	if !ok {
		return nil
	}

	lockUpdated := false
	if parent.View() > hs.locked.View() {
		hs.locked = parent
		lockUpdated = true
		hs.logger.Debug("LOCK: ", parent)
	}

	// Persist lock update
	if lockUpdated {
		hs.persistLocked()
	}

	grandparent, ok := hs.blockChain.Get(parent.QuorumCert().BlockHash())
	if !ok {
		return nil
	}

	// if there are two consecutive blocks, then the first one is committed
	if parent.Parent() == grandparent.Hash() && parent.View() == grandparent.View()+1 {
		hs.logger.Debug("COMMIT: ", grandparent)
		return grandparent
	}

	return nil
}

// ChainLength returns the number of blocks that need to be chained together in order to commit.
func (hs *PersistentSimpleHotStuff) ChainLength() int {
	return 2
}

// Close cleanly shuts down the persistent SimpleHotStuff
func (hs *PersistentSimpleHotStuff) Close() error {
	if hs.stateStore != nil {
		return hs.stateStore.Close()
	}
	return nil
}

var _ consensus.Rules = (*PersistentSimpleHotStuff)(nil)
