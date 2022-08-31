// Copyright 2022 The go-fantom Authors
// This file is part of the go-fantom library.
//
// The go-fantom library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package vm

import (
	"database/sql"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	_ "github.com/mattn/go-sqlite3"
	"log"
	"math/big"
	"sync"
	"time"
)

type BasicBlockKey struct {
	Contract string
	Instructions string
	Address uint64
}

// VM Micro Dataset for profiling
type VmMicroData struct {
	opCodeFrequency      map[OpCode]big.Int                   // opcode frequency statistics
	opCodeDuration       map[OpCode]big.Int                   // accumulated duration of opcodes
	instructionFrequency map[uint64]big.Int                   // instruction frequency statistics
	stepLengthFrequency  map[int]big.Int                      // smart contract length frequency
	basicBlockFrequency  map[BasicBlockKey]uint64             // basic block statistics
	isInitialized        bool
	mx                   sync.Mutex // mutex to protect micro dataset
}

// single global data set for all workers
var vmStats VmMicroData

func (d *VmMicroData) Initialize() {
	d.mx.Lock()
	if !d.isInitialized {
		d.opCodeFrequency = make(map[OpCode]big.Int)
		d.opCodeDuration = make(map[OpCode]big.Int)
		d.instructionFrequency = make(map[uint64]big.Int)
		d.stepLengthFrequency = make(map[int]big.Int)
		d.basicBlockFrequency = make(map[BasicBlockKey]uint64)
		d.isInitialized = true
	}
	d.mx.Unlock()
}

// update statistics
func (d *VmMicroData) UpdateStatistics(contract *common.Address, opCodeFrequency map[OpCode]uint64, opCodeDuration map[OpCode]time.Duration, instructionFrequency map[uint64]uint64, basicBlockFrequency map[uint64]BasicBlock, stepLength int) {
	// get access to dataset
	d.mx.Lock()

	// update opcode frequency
	for opCode, freq := range opCodeFrequency {
		value := d.opCodeFrequency[opCode]
		value.Add(&value, new(big.Int).SetUint64(uint64(freq)))
		d.opCodeFrequency[opCode] = value
	}

	// update instruction opCodeDuration
	for opCode, duration := range opCodeDuration {
		value := d.opCodeDuration[opCode]
		value.Add(&value, new(big.Int).SetUint64(uint64(duration)))
		d.opCodeDuration[opCode] = value
	}

	// update instruction frequency
	for instruction, freq := range instructionFrequency {
		value := d.instructionFrequency[instruction]
		value.Add(&value, new(big.Int).SetUint64(uint64(freq)))
		d.instructionFrequency[instruction] = value
	}

	// update jump destination frequency
	for addr, bb := range basicBlockFrequency {
		bkey := BasicBlockKey {}
		bkey.Contract = contract.String()
		bkey.Address = addr
		bkey.Instructions = string(bb.Instructions)
		d.basicBlockFrequency[bkey] += bb.Frequency
	}

	// step length frequency
	value := d.stepLengthFrequency[stepLength]
	value.Add(&value, new(big.Int).SetUint64(uint64(1)))
	d.stepLengthFrequency[stepLength] = value
	// release data set
	d.mx.Unlock()
}

// update statistics
func PrintStatistics() {
	// get access to dataset
	vmStats.mx.Lock()

	// print opcode frequency
	for opCode, freq := range vmStats.opCodeFrequency {
		fmt.Printf("opcode-freq: %v,%v\n", opCode, freq.String())
	}

	// print total opcode duration in seconds
	for opCode, duration := range vmStats.opCodeDuration {
		seconds := new(big.Int)
		seconds.Div(&duration, big.NewInt(int64(1000000000)))
		fmt.Printf("opcode-runtime-total-s: %v,%v\n", opCodeToString[opCode], seconds.String())
		average := new(big.Int)
		divisor := vmStats.opCodeFrequency[opCode]
		average.Div(&duration, &divisor)
		fmt.Printf("opcode-runtime-avg-ns: %v,%v\n", opCodeToString[opCode], average.String())
	}

	// print instruction frequency
	for instruction, freq := range vmStats.instructionFrequency {
		fmt.Printf("instruction-freq: %v,%v\n", instruction, freq.String())
	}

	// print step-length frequency
	for length, freq := range vmStats.stepLengthFrequency {
		fmt.Printf("steplen-freq: %v,%v\n", length, freq.String())
	}

	// Dump jump destination frequency statistics into a SQLITE3 database

	// open sqlite3 database
	db, err := sql.Open("sqlite3", "./jumpdest.db") // Open the created SQLite File
	if err != nil {
		log.Fatal(err.Error())
	}
	defer db.Close()

	// drop jump-dest table
	const dropBasicBlockFrequency string = `DROP TABLE IF EXISTS BasicBlockFrequency;`
	statement, err := db.Prepare(dropBasicBlockFrequency)
	if err != nil {
		log.Fatal(err.Error())
	}
	_, err = statement.Exec()
	if err != nil {
		log.Fatalln(err.Error())
	}

	// create new table
	const createBasicBlockFrequency string = `
	CREATE TABLE BasicBlockFrequency (
	 contract TEXT,
	 address NUMERIC,
	 instructions BLOB,
	 frequency NUMERIC
	);`
	statement, err = db.Prepare(createBasicBlockFrequency)
	if err != nil {
		log.Fatal(err.Error())
	}
	_, err = statement.Exec()
	if err != nil {
		log.Fatalln(err.Error())
	}

	// populate values
	insertFrequency := `INSERT INTO BasicBlockFrequency(contract, address, instructions, frequency) VALUES (?, ?, ?, ?)`
	statement, err = db.Prepare(insertFrequency)
	if err != nil {
		log.Fatalln(err.Error())
	}
	for bkey, freq := range vmStats.basicBlockFrequency {
		_, err = statement.Exec(bkey.Contract, bkey.Address, []byte(bkey.Instructions), freq)
		if err != nil {
			log.Fatalln(err.Error())
		}
	}

	// release data set
	vmStats.mx.Unlock()
}
