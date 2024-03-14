package state

import (
	"encoding/json"
	substate "github.com/Fantom-foundation/Substate"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state/snapshot"
	"github.com/ethereum/go-ethereum/core/types"
	"math/big"
	"time"
)

type StateDbInterface interface {
	StartPrefetcher(namespace string)
	StopPrefetcher()
	Error() error
	AddLog(log *types.Log)
	GetLogs(hash common.Hash, blockHash common.Hash) []*types.Log
	Logs() []*types.Log
	AddPreimage(hash common.Hash, preimage []byte)
	Preimages() map[common.Hash][]byte
	AddRefund(gas uint64)
	SubRefund(gas uint64)
	Exist(addr common.Address) bool
	Empty(addr common.Address) bool
	GetBalance(addr common.Address) *big.Int
	GetNonce(addr common.Address) uint64
	TxIndex() int
	GetCode(addr common.Address) []byte
	GetCodeSize(addr common.Address) int
	GetCodeHash(addr common.Address) common.Hash
	GetState(addr common.Address, hash common.Hash) common.Hash
	GetProof(addr common.Address) ([][]byte, error)
	GetProofByHash(addrHash common.Hash) ([][]byte, error)
	GetStorageProof(a common.Address, key common.Hash) ([][]byte, error)
	GetCommittedState(addr common.Address, hash common.Hash) common.Hash
	Database() Database
	StorageTrie(addr common.Address) Trie
	HasSuicided(addr common.Address) bool
	AddBalance(addr common.Address, amount *big.Int)
	SubBalance(addr common.Address, amount *big.Int)
	SetBalance(addr common.Address, amount *big.Int)
	SetNonce(addr common.Address, nonce uint64)
	SetCode(addr common.Address, code []byte)
	SetState(addr common.Address, key, value common.Hash)
	SetStorage(addr common.Address, storage map[common.Hash]common.Hash)
	Suicide(addr common.Address) bool
	CreateAccount(addr common.Address)
	ForEachStorage(addr common.Address, cb func(key, value common.Hash) bool) error
	Copy() StateDbInterface
	Snapshot() int
	RevertToSnapshot(revid int)
	GetRefund() uint64
	Finalise(deleteEmptyObjects bool)
	IntermediateRoot(deleteEmptyObjects bool) common.Hash
	Prepare(thash common.Hash, ti int)
	Commit(deleteEmptyObjects bool) (common.Hash, error)
	PrepareAccessList(sender common.Address, dst *common.Address, precompiles []common.Address, list types.AccessList)
	AddAddressToAccessList(addr common.Address)
	AddSlotToAccessList(addr common.Address, slot common.Hash)
	AddressInAccessList(addr common.Address) bool
	SlotInAccessList(addr common.Address, slot common.Hash) (addressPresent bool, slotPresent bool)

	RawDump(opts *DumpConfig) Dump
	IteratorDump(opts *DumpConfig) IteratorDump
	IterativeDump(opts *DumpConfig, output *json.Encoder)
	Dump(opts *DumpConfig) []byte
	DumpToCollector(c DumpCollector, conf *DumpConfig) (nextKey []byte)

	GetAccountReads() time.Duration
	GetAccountHashes() time.Duration
	GetAccountUpdates() time.Duration
	GetAccountCommits() time.Duration
	GetStorageReads() time.Duration
	GetStorageHashes() time.Duration
	GetStorageUpdates() time.Duration
	GetStorageCommits() time.Duration
	GetSnapshotAccountReads() time.Duration
	GetSnapshotStorageReads() time.Duration
	GetSnapshotCommits() time.Duration

	SetPrehashedCode(addr common.Address, hash common.Hash, code []byte)
	GetSubstatePostAlloc() substate.SubstateAlloc
	BeginBlock(number uint64)
	EndBlock(number uint64)
	Release()
}

type StateDB struct {
	StateDbInterface

	// Measurements gathered during execution for debugging purposes
	AccountReads         time.Duration
	AccountHashes        time.Duration
	AccountUpdates       time.Duration
	AccountCommits       time.Duration
	StorageReads         time.Duration
	StorageHashes        time.Duration
	StorageUpdates       time.Duration
	StorageCommits       time.Duration
	SnapshotAccountReads time.Duration
	SnapshotStorageReads time.Duration
	SnapshotCommits      time.Duration
}

func (s *StateDB) Copy() *StateDB {
	return &StateDB{s.StateDbInterface.Copy(), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
}

func (s *StateDB) Commit(deleteEmptyObjects bool) (common.Hash, error) {
	hash, err := s.StateDbInterface.Commit(deleteEmptyObjects)
	s.AccountReads = s.GetAccountReads()
	s.AccountHashes = s.GetAccountHashes()
	s.AccountUpdates = s.GetAccountUpdates()
	s.AccountCommits = s.GetAccountCommits()
	s.StorageReads = s.GetStorageReads()
	s.StorageHashes = s.GetStorageHashes()
	s.StorageUpdates = s.GetStorageUpdates()
	s.StorageCommits = s.GetStorageCommits()
	s.SnapshotAccountReads = s.GetSnapshotAccountReads()
	s.SnapshotStorageReads = s.GetSnapshotStorageReads()
	s.SnapshotCommits = s.GetSnapshotCommits()
	return hash, err
}

// New creates a new state from a given trie.
func New(root common.Hash, db Database, snaps *snapshot.Tree) (*StateDB, error) {
	return NewWithSnapLayers(root, db, snaps, 128)
}

func NewWithSnapLayers(root common.Hash, db Database, snaps *snapshot.Tree, layers int) (*StateDB, error) {
	sdb, err := NewLegacyWithSnapLayers(root, db, snaps, layers)
	if err != nil {
		return nil, err
	}
	return NewWrapper(sdb), nil
}

func NewWrapper(inner StateDbInterface) *StateDB {
	return &StateDB{inner, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
}
