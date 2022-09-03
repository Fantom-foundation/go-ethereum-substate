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
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	_ "github.com/mattn/go-sqlite3"
	"log"
	"math/big"
	"time"
)

// Key for a basic-block for frequency measurement
type BasicBlockKey struct {
	Contract     string // contract in hex format
	Instructions string // instructions in hex format
	Address      uint64 // basic-block start address
}

// Data record for a single smart contract invocation
type SmartContractData struct {
	Contract             common.Address           // smart contract address
	OpCodeFrequency      map[OpCode]uint64        // opcode frequency stats
	OpCodeDuration       map[OpCode]time.Duration // opcode durations stats
	InstructionFrequency map[uint64]uint64        // instruction frequency stats
	BasicBlockFrequency  map[uint64]BasicBlock    // basic block frequency
	StepLength           int                      // number of executed instructions
}

// VM Micro Dataset for profiling
type VmMicroData struct {
	opCodeFrequency      map[OpCode]big.Int       // opcode frequency statistics
	opCodeDuration       map[OpCode]big.Int       // accumulated duration of opcodes
	instructionFrequency map[uint64]big.Int       // instruction frequency statistics
	stepLengthFrequency  map[int]big.Int          // smart contract length frequency
	basicBlockFrequency  map[BasicBlockKey]uint64 // basic block statistics
}

// single global data set for whole blockchain
var vmStats VmMicroData = VmMicroData{
	opCodeFrequency:      make(map[OpCode]big.Int),
	opCodeDuration:       make(map[OpCode]big.Int),
	instructionFrequency: make(map[uint64]big.Int),
	stepLengthFrequency:  make(map[int]big.Int),
	basicBlockFrequency:  make(map[BasicBlockKey]uint64),
}

// Maximal number of records per SQLITE3 transaction 
// for dumping basic block statistics
const BasicBlockMaxNumRecords = 1000

// Record Queue to store smart-contract data records from each worker
// This is a lock-free queue; hence sufficiently fast
var recordQueue *RecordQueue = NewRecordQueue()

// Flush the record queue
func FlushQueue() {
	for {
		// pop all records and process them
		if scd := recordQueue.Dequeue(); scd != nil {
			// update opcode frequency
			for opCode, freq := range scd.OpCodeFrequency {
				value := vmStats.opCodeFrequency[opCode]
				value.Add(&value, new(big.Int).SetUint64(uint64(freq)))
				vmStats.opCodeFrequency[opCode] = value
			}

			// update instruction opCodeDuration
			for opCode, duration := range scd.OpCodeDuration {
				value := vmStats.opCodeDuration[opCode]
				value.Add(&value, new(big.Int).SetUint64(uint64(duration)))
				vmStats.opCodeDuration[opCode] = value
			}

			// update instruction frequency
			for instruction, freq := range scd.InstructionFrequency {
				value := vmStats.instructionFrequency[instruction]
				value.Add(&value, new(big.Int).SetUint64(uint64(freq)))
				vmStats.instructionFrequency[instruction] = value
			}

			// update basic block frequency
			for addr, bb := range scd.BasicBlockFrequency {
				bkey := BasicBlockKey{Contract: scd.Contract.String(), Address: addr, Instructions: hex.EncodeToString(bb.Instructions)}
				vmStats.basicBlockFrequency[bkey] += bb.Frequency
			}

			// step length frequency
			value := vmStats.stepLengthFrequency[scd.StepLength]
			value.Add(&value, new(big.Int).SetUint64(uint64(1)))
			vmStats.stepLengthFrequency[scd.StepLength] = value
		} else {
			break
		}
	}
}

// The data collector checks for a stopping signal and flushes
// the record queue of the workers. It is a background task
func DataCollector(ctx context.Context, done chan struct{}) {
	defer close(done)

	// check for stop signals and process queue
	for {
		// received stop signal?
		select {
		case <-ctx.Done():
			return
		default:
			// flush queue
			FlushQueue()

			// send worker to sleep (avoiding busy waiting)
			// time.Sleep(1 * time.Millisecond)
		}
	}
}

// put record into queue for later processing by collector worker
func ProcessSmartContractData(scd *SmartContractData) {
	// don't process now; put it into queue
	recordQueue.Enqueue(scd)
}

// dump basic block frequency stats into a SQLITE3 database
func DumpBasicBlockFrequency() {
	// Dump basic-block frequency statistics into a SQLITE3 database

	// open sqlite3 database
	db, err := sql.Open("sqlite3", "./basicblocks.db") // Open the created SQLite File
	if err != nil {
		log.Fatal(err.Error())
	}
	defer db.Close()

	// drop basic-block frequency table
	const dropBasicBlockFrequency string = `DROP TABLE IF EXISTS BasicBlockFrequency;`
	_, err = db.Exec(dropBasicBlockFrequency)
	if err != nil {
		log.Fatalln(err.Error())
	}

	// create new table
	const createBasicBlockFrequency string = `
	CREATE TABLE BasicBlockFrequency (
	 contract TEXT,
	 address NUMERIC,
	 instructions TEXT,
	 frequency NUMERIC
	);`
	_, err = db.Exec(createBasicBlockFrequency)
	if err != nil {
		log.Fatalln(err.Error())
	}

	// switch synchronous mode off, enable memory journaling,
	// and start a new transaction
	_, err = db.Exec("PRAGMA synchronous = OFF;PRAGMA journal_mode = MEMORY;BEGIN TRANSACTION")
	if err != nil {
		log.Fatalln(err.Error())
	}

	// prepare the insert statement for faster inserts
	insertFrequency := `INSERT INTO BasicBlockFrequency(contract, address, instructions, frequency) VALUES (?, ?, ?, ?)`
	statement, err := db.Prepare(insertFrequency)
	if err != nil {
		log.Fatalln(err.Error())
	}

	// populate all values into the DB
	ctr := 1
	for bkey, freq := range vmStats.basicBlockFrequency {
		// commit dataset when record threshold is reached
		if ctr >= BasicBlockMaxNumRecords {
			ctr = 1
			_, err = db.Exec("END TRANSACTION; BEGIN TRANSACTION;")
			if err != nil {
				log.Fatalln(err.Error())
			}
		} else {
			ctr++
		}
		_, err = statement.Exec(bkey.Contract, bkey.Address, bkey.Instructions, freq)
		if err != nil {
			log.Fatalln(err.Error())
		}

	}

	// end transaction
	_, err = db.Exec("END TRANSACTION;")
	if err != nil {
		log.Fatalln(err.Error())
	}
}

// update statistics
func PrintStatistics() {
	// flush queue; ensure that we don't lose records
	FlushQueue()

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

	// Dump basic block frequency stats into a SQLITE database
	DumpBasicBlockFrequency()
}
