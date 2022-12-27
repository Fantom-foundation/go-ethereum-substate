package substate

import (
	"encoding/binary"
	"fmt"
	"log"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/rlp"
)

type DestroyedAccountDB struct {
	backend BackendDatabase
}

func NewDestroyedAccountDB(backend BackendDatabase) *DestroyedAccountDB {
	return &DestroyedAccountDB{backend: backend}
}

func OpenDestroyedAccountDB(destroyedAccountDir string) *DestroyedAccountDB {
	return openDestroyedAccountDB(destroyedAccountDir, false)
}

func OpenDestroyedAccountDBReadOnly(destroyedAccountDir string) *DestroyedAccountDB {
	return openDestroyedAccountDB(destroyedAccountDir, true)
}

func openDestroyedAccountDB(destroyedAccountDir string, readOnly bool) *DestroyedAccountDB {
	log.Println("substate: OpenDestroyedAccountDB")
	backend, err := rawdb.NewLevelDBDatabase(destroyedAccountDir, 1024, 100, "destroyed_accounts")
	if err != nil {
		panic(fmt.Errorf("error opening destroyed account leveldb %s: %v", destroyedAccountDir, err))
	}
	return NewDestroyedAccountDB(backend)
}

func (db *DestroyedAccountDB) Close() error {
	return db.backend.Close()
}

type SuicidedAccountLists struct {
	DestroyedAccounts   []common.Address
	ResurrectedAccounts []common.Address
}

func (db *DestroyedAccountDB) SetDestroyedAccounts(block uint64, des []common.Address, res []common.Address) error {
	accountList := SuicidedAccountLists{DestroyedAccounts: des, ResurrectedAccounts: res}
	value, err := rlp.EncodeToBytes(accountList)
	if err != nil {
		panic(err)
	}
	return db.backend.Put(encodeDestroyedAccountKey(block), value)
}

func (db *DestroyedAccountDB) GetDestroyedAccounts(block uint64) (SuicidedAccountLists, error) {
	data, err := db.backend.Get(encodeDestroyedAccountKey(block))
	if err != nil {
		panic(err)
	}
	return decodeAddressList(data)
}

// GetAccountsDestroyedInRange get list of all accounts between block from and to (including from and to).
func (db *DestroyedAccountDB) GetAccountsDestroyedInRange(from, to uint64) ([]common.Address, error) {
	iter := db.backend.NewIterator(nil, encodeDestroyedAccountKey(from))
	defer iter.Release()
	isDestroyed := make(map[common.Address]bool)
	for iter.Next() {
		block, err := decodeDestroyedAccountKey(iter.Key())
		if err != nil {
			return nil, err
		}
		if block > to {
			break
		}
		list, err := decodeAddressList(iter.Value())
		if err != nil {
			return nil, err
		}
		for _, addr := range list.DestroyedAccounts {
			isDestroyed[addr] = true
		}
		for _, addr := range list.ResurrectedAccounts {
			isDestroyed[addr] = false
		}
	}

	accountList := []common.Address{}
	for addr, isDeleted := range isDestroyed {
		if isDeleted {
			accountList = append(accountList, addr)
		}
	}
	return accountList, nil
}

const (
	destroyedAccountPrefix = "da" // destroyedAccountPrefix + block (64-bit) -> SuicidedAccountLists
)

func encodeDestroyedAccountKey(block uint64) []byte {
	prefix := []byte(destroyedAccountPrefix)
	key := make([]byte, len(prefix)+8)
	copy(key[0:], prefix)
	binary.BigEndian.PutUint64(key[len(prefix):], block)
	return key
}

func decodeDestroyedAccountKey(data []byte) (uint64, error) {
	if len(data) != len(destroyedAccountPrefix)+8 {
		return 0, fmt.Errorf("invalid length of destroyed account key, expected %d, got %d", len(destroyedAccountPrefix)+8, len(data))
	}
	if string(data[0:len(destroyedAccountPrefix)]) != destroyedAccountPrefix {
		return 0, fmt.Errorf("invalid prefix of destroyed account key")
	}
	return binary.BigEndian.Uint64(data[len(destroyedAccountPrefix):]), nil
}

func decodeAddressList(data []byte) (SuicidedAccountLists, error) {
	list := SuicidedAccountLists{}
	err := rlp.DecodeBytes(data, &list)
	return list, err
}
