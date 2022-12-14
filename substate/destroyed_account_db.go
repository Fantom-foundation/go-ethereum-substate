package substate

import (
	"encoding/binary"
	"fmt"
	"log"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
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
	backend, err := rawdb.NewLevelDBDatabase(destroyedAccountDir, 1024, 100, "destroyed_accounts", readOnly)
	if err != nil {
		panic(fmt.Errorf("error opening destroyed account leveldb %s: %v", destroyedAccountDir, err))
	}
	return NewDestroyedAccountDB(backend)
}

func (db *DestroyedAccountDB) Close() error {
	return db.backend.Close()
}

func (db *DestroyedAccountDB) SetDestroyedAccounts(block uint64, accounts []common.Address) error {
	return db.backend.Put(encodeDestroyedAccountKey(block), encodeAddressList(accounts))
}

func (db *DestroyedAccountDB) GetDestroyedAccounts(block uint64) ([]common.Address, error) {
	data, err := db.backend.Get(encodeDestroyedAccountKey(block))
	if err != nil {
		return nil, err
	}
	return decodeAddressList(data)
}

// GetAccountsDestroyedInRange get list of all accounts between block from and to (including from and to).
func (db *DestroyedAccountDB) GetAccountsDestroyedInRange(from, to uint64) ([]common.Address, error) {
	iter := db.backend.NewIterator(nil, encodeDestroyedAccountKey(from))
	defer iter.Release()
	res := []common.Address{}
	for iter.Next() {
		block, err := decodeDestroyedAccountKey(iter.Key())
		if err != nil {
			return nil, err
		}
		if block > to {
			return res, nil
		}
		list, err := decodeAddressList(iter.Value())
		if err != nil {
			return nil, err
		}
		res = append(res, list...)
	}
	return res, nil
}

const (
	destroyedAccountPrefix = "da" // destroyedAccountPrefix + block (64-bit) -> []common.Address
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
	return binary.BigEndian.Uint64(data[2:]), nil
}

func encodeAddressList(list []common.Address) []byte {
	res := make([]byte, len(list)*common.AddressLength)
	for i := range list {
		copy(res[i*common.AddressLength:], list[i][:])
	}
	return res
}

func decodeAddressList(data []byte) ([]common.Address, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if len(data)%common.AddressLength != 0 {
		return nil, fmt.Errorf("invalid lenght of address list encoding: %d", len(data))
	}
	numAddresses := len(data) / common.AddressLength
	res := make([]common.Address, numAddresses)
	for i := 0; i < numAddresses; i++ {
		copy(res[i][:], data[i*common.AddressLength:])
	}
	return res, nil
}
