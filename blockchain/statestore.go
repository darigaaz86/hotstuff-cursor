package blockchain

import (
	"encoding/binary"
	"fmt"
	"path/filepath"

	"github.com/dgraph-io/badger/v4"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"google.golang.org/protobuf/proto"
)

// State store keys
const (
	stateCurrentView   = "state:current_view"
	stateLastVote      = "state:last_vote"
	stateCommittedHash = "state:committed_hash"
	stateHighQC        = "state:high_qc"
	stateHighTC        = "state:high_tc"
	stateLockHash      = "state:lock_hash" // for consensus algorithms that maintain a locked block
)

// StateStore manages persistent consensus and synchronizer state
type StateStore struct {
	db *badger.DB
}

// NewStateStore creates a new persistent state store
func NewStateStore(dataDir string) (*StateStore, error) {
	dbPath := filepath.Join(dataDir, "state.db")

	opts := badger.DefaultOptions(dbPath).
		WithLogger(nil)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open state database: %w", err)
	}

	store := &StateStore{db: db}

	// Initialize with default values if empty
	if err := store.initializeDefaults(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize defaults: %w", err)
	}

	return store, nil
}

// Close closes the state database
func (s *StateStore) Close() error {
	return s.db.Close()
}

// Consensus State Management

// GetCurrentView returns the current consensus view
func (s *StateStore) GetCurrentView() (hotstuff.View, error) {
	var view hotstuff.View
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(stateCurrentView))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				view = 1 // default starting view
				return nil
			}
			return err
		}

		return item.Value(func(val []byte) error {
			if len(val) != 8 {
				return fmt.Errorf("invalid view length: %d", len(val))
			}
			view = hotstuff.View(binary.LittleEndian.Uint64(val))
			return nil
		})
	})
	return view, err
}

// SetCurrentView saves the current consensus view
func (s *StateStore) SetCurrentView(view hotstuff.View) error {
	return s.db.Update(func(txn *badger.Txn) error {
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], uint64(view))
		return txn.Set([]byte(stateCurrentView), buf[:])
	})
}

// GetLastVote returns the last vote view
func (s *StateStore) GetLastVote() (hotstuff.View, error) {
	var view hotstuff.View
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(stateLastVote))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				view = 0 // default
				return nil
			}
			return err
		}

		return item.Value(func(val []byte) error {
			if len(val) != 8 {
				return fmt.Errorf("invalid last vote length: %d", len(val))
			}
			view = hotstuff.View(binary.LittleEndian.Uint64(val))
			return nil
		})
	})
	return view, err
}

// SetLastVote saves the last vote view
func (s *StateStore) SetLastVote(view hotstuff.View) error {
	return s.db.Update(func(txn *badger.Txn) error {
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], uint64(view))
		return txn.Set([]byte(stateLastVote), buf[:])
	})
}

// GetCommittedBlockHash returns the hash of the last committed block
func (s *StateStore) GetCommittedBlockHash() (hotstuff.Hash, error) {
	var hash hotstuff.Hash
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(stateCommittedHash))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				// Return genesis hash as default
				hash = hotstuff.GetGenesis().Hash()
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
	return hash, err
}

// SetCommittedBlockHash saves the hash of the last committed block
func (s *StateStore) SetCommittedBlockHash(hash hotstuff.Hash) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(stateCommittedHash), hash[:])
	})
}

// Certificate Management

// GetHighQC returns the highest known quorum certificate
func (s *StateStore) GetHighQC() (hotstuff.QuorumCert, error) {
	var qc hotstuff.QuorumCert
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(stateHighQC))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				// Return empty QC as default
				qc = hotstuff.NewQuorumCert(nil, 0, hotstuff.Hash{})
				return nil
			}
			return err
		}

		return item.Value(func(val []byte) error {
			var pbQC hotstuffpb.QuorumCert
			if err := proto.Unmarshal(val, &pbQC); err != nil {
				return fmt.Errorf("failed to unmarshal QC: %w", err)
			}
			qc = hotstuffpb.QuorumCertFromProto(&pbQC)
			return nil
		})
	})
	return qc, err
}

// SetHighQC saves the highest known quorum certificate
func (s *StateStore) SetHighQC(qc hotstuff.QuorumCert) error {
	return s.db.Update(func(txn *badger.Txn) error {
		pbQC := hotstuffpb.QuorumCertToProto(qc)
		data, err := proto.Marshal(pbQC)
		if err != nil {
			return fmt.Errorf("failed to marshal QC: %w", err)
		}
		return txn.Set([]byte(stateHighQC), data)
	})
}

// GetHighTC returns the highest known timeout certificate
func (s *StateStore) GetHighTC() (hotstuff.TimeoutCert, error) {
	var tc hotstuff.TimeoutCert
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(stateHighTC))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				// Return empty TC as default
				tc = hotstuff.NewTimeoutCert(nil, 0)
				return nil
			}
			return err
		}

		return item.Value(func(val []byte) error {
			var pbTC hotstuffpb.TimeoutCert
			if err := proto.Unmarshal(val, &pbTC); err != nil {
				return fmt.Errorf("failed to unmarshal TC: %w", err)
			}
			tc = hotstuffpb.TimeoutCertFromProto(&pbTC)
			return nil
		})
	})
	return tc, err
}

// SetHighTC saves the highest known timeout certificate
func (s *StateStore) SetHighTC(tc hotstuff.TimeoutCert) error {
	return s.db.Update(func(txn *badger.Txn) error {
		pbTC := hotstuffpb.TimeoutCertToProto(tc)
		data, err := proto.Marshal(pbTC)
		if err != nil {
			return fmt.Errorf("failed to marshal TC: %w", err)
		}
		return txn.Set([]byte(stateHighTC), data)
	})
}

// Lock Management (for consensus algorithms)

// GetLockedBlockHash returns the hash of the currently locked block
func (s *StateStore) GetLockedBlockHash() (hotstuff.Hash, error) {
	var hash hotstuff.Hash
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(stateLockHash))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				// Return genesis hash as default
				hash = hotstuff.GetGenesis().Hash()
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
	return hash, err
}

// SetLockedBlockHash saves the hash of the currently locked block
func (s *StateStore) SetLockedBlockHash(hash hotstuff.Hash) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(stateLockHash), hash[:])
	})
}

// Helper Methods

// initializeDefaults sets up default values if the database is empty
func (s *StateStore) initializeDefaults() error {
	return s.db.Update(func(txn *badger.Txn) error {
		// Initialize current view if not exists
		if _, err := txn.Get([]byte(stateCurrentView)); err == badger.ErrKeyNotFound {
			var buf [8]byte
			binary.LittleEndian.PutUint64(buf[:], uint64(1))
			if err := txn.Set([]byte(stateCurrentView), buf[:]); err != nil {
				return err
			}
		}

		// Initialize last vote if not exists
		if _, err := txn.Get([]byte(stateLastVote)); err == badger.ErrKeyNotFound {
			var buf [8]byte
			binary.LittleEndian.PutUint64(buf[:], uint64(0))
			if err := txn.Set([]byte(stateLastVote), buf[:]); err != nil {
				return err
			}
		}

		// Initialize committed block hash if not exists
		if _, err := txn.Get([]byte(stateCommittedHash)); err == badger.ErrKeyNotFound {
			genesis := hotstuff.GetGenesis()
			hash := genesis.Hash()
			if err := txn.Set([]byte(stateCommittedHash), hash[:]); err != nil {
				return err
			}
		}

		// Initialize locked block hash if not exists
		if _, err := txn.Get([]byte(stateLockHash)); err == badger.ErrKeyNotFound {
			genesis := hotstuff.GetGenesis()
			hash := genesis.Hash()
			if err := txn.Set([]byte(stateLockHash), hash[:]); err != nil {
				return err
			}
		}

		return nil
	})
}

// Atomic State Updates

// UpdateConsensusState atomically updates multiple consensus state fields
func (s *StateStore) UpdateConsensusState(updates map[string]interface{}) error {
	return s.db.Update(func(txn *badger.Txn) error {
		for key, value := range updates {
			switch key {
			case "current_view":
				if view, ok := value.(hotstuff.View); ok {
					var buf [8]byte
					binary.LittleEndian.PutUint64(buf[:], uint64(view))
					if err := txn.Set([]byte(stateCurrentView), buf[:]); err != nil {
						return err
					}
				}
			case "last_vote":
				if view, ok := value.(hotstuff.View); ok {
					var buf [8]byte
					binary.LittleEndian.PutUint64(buf[:], uint64(view))
					if err := txn.Set([]byte(stateLastVote), buf[:]); err != nil {
						return err
					}
				}
			case "committed_hash":
				if hash, ok := value.(hotstuff.Hash); ok {
					if err := txn.Set([]byte(stateCommittedHash), hash[:]); err != nil {
						return err
					}
				}
			case "lock_hash":
				if hash, ok := value.(hotstuff.Hash); ok {
					if err := txn.Set([]byte(stateLockHash), hash[:]); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
}
