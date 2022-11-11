package substate

import (
	"encoding/binary"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	SubstateAllocPrefix = "1s" // SubstateAllocPrefix + block (64-bit) + tx (64-bit) -> substateRLP
	SubstateAllocCodePrefix     = "1c" // SubstateAllocCodePrefix + codeHash (256-bit) -> code
)

func SubstateAllocKey(block uint64) []byte {
	prefix := []byte(SubstateAllocPrefix)
	blockTx := make([]byte, 16)
	binary.BigEndian.PutUint64(blockTx[0:8], block)
	return append(prefix, blockTx...)
}

func DecodeSubstateAllocKey(key []byte) (block uint64, err error) {
	prefix := SubstateAllocPrefix
	if len(key) != len(prefix)+8 {
		err = fmt.Errorf("invalid length of stage1 substate key: %v", len(key))
		return
	}
	if p := string(key[:len(prefix)]); p != prefix {
		err = fmt.Errorf("invalid prefix of stage1 substate key: %#x", p)
		return
	}
	blockTx := key[len(prefix):]
	block = binary.BigEndian.Uint64(blockTx[0:8])
	return
}

func SubstateAllocBlockPrefix(block uint64) []byte {
	prefix := []byte(SubstateAllocPrefix)

	blockBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(blockBytes[0:8], block)

	return append(prefix, blockBytes...)
}

type UpdateDB struct {
	backend BackendDatabase
}

func NewUpdateDB(backend BackendDatabase) *UpdateDB {
	return &UpdateDB{backend: backend}
}

func (db *UpdateDB) Compact(start []byte, limit []byte) error {
	return db.backend.Compact(start, limit)
}

func (db *UpdateDB) Close() error {
	return db.backend.Close()
}

func (db *UpdateDB) HasCode(codeHash common.Hash) bool {
	if codeHash == EmptyCodeHash {
		return false
	}
	key := Stage1CodeKey(codeHash)
	has, err := db.backend.Has(key)
	if err != nil {
		panic(fmt.Errorf("record-replay: error checking bytecode for codeHash %s: %v", codeHash.Hex(), err))
	}
	return has
}

func (db *UpdateDB) GetCode(codeHash common.Hash) []byte {
	if codeHash == EmptyCodeHash {
		return nil
	}
	key := Stage1CodeKey(codeHash)
	code, err := db.backend.Get(key)
	if err != nil {
		panic(fmt.Errorf("record-replay: error getting code %s: %v", codeHash.Hex(), err))
	}
	return code
}

func (db *UpdateDB) PutCode(code []byte) {
	if len(code) == 0 {
		return
	}
	codeHash := crypto.Keccak256Hash(code)
	key := Stage1CodeKey(codeHash)
	err := db.backend.Put(key, code)
	if err != nil {
		panic(fmt.Errorf("record-replay: error putting code %s: %v", codeHash.Hex(), err))
	}
}

func (db *UpdateDB) HasUpdateSet(block uint64) bool {
	key := SubstateAllocKey(block)
	has, _ := db.backend.Has(key)
	return has
}

func (alloc *SubstateAlloc) SetRLP2(allocRLP SubstateAllocRLP, db *UpdateDB) {
	*alloc = make(SubstateAlloc)
	for i, addr := range allocRLP.Addresses {
		var sa SubstateAccount

		saRLP := allocRLP.Accounts[i]
		sa.Balance = saRLP.Balance
		sa.Nonce = saRLP.Nonce
		sa.Code = db.GetCode(saRLP.CodeHash)
		sa.Storage = make(map[common.Hash]common.Hash)
		for i := range saRLP.Storage {
			sa.Storage[saRLP.Storage[i][0]] = saRLP.Storage[i][1]
		}

		(*alloc)[addr] = &sa
	}
}

func (db *UpdateDB) GetUpdateSet(block uint64) *SubstateAlloc {
	var err error
	key := SubstateAllocKey(block)
	value, err := db.backend.Get(key)
	if err != nil {
		panic(fmt.Errorf("record-replay: error getting substate %v from substate DB: %v,", block, err))
	}
	// try decoding as substates from latest hard forks
	updateSetRLP := SubstateAllocRLP{}
	err = rlp.DecodeBytes(value, &updateSetRLP)
	updateSet := SubstateAlloc{}
	updateSet.SetRLP2(updateSetRLP, db)
	return &updateSet
}

func (db *UpdateDB) PutUpdateSet(block uint64, updateSet *SubstateAlloc) {
	var err error

	// put deployed/creation code
	for _, account := range *updateSet {
		db.PutCode(account.Code)
	}
	key := SubstateAllocKey(block)
	defer func() {
		if err != nil {
			panic(fmt.Errorf("record-replay: error putting update-set %v into substate DB: %v", block,  err))
		}
	}()

	updateSetRLP := NewSubstateAllocRLP(*updateSet)
	value, err := rlp.EncodeToBytes(updateSetRLP)
	if err != nil {
		panic(err)
	}
	err = db.backend.Put(key, value)
	if err != nil {
		panic(err)
	}
}

func (db *UpdateDB) DeleteSubstateAlloc(block uint64) {
	key := SubstateAllocKey(block)
	err := db.backend.Delete(key)
	if err != nil {
		panic(err)
	}
}

