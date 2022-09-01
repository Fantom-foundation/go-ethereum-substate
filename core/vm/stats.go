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
	"encoding/hex"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	_ "github.com/mattn/go-sqlite3"
	"log"
	"math/big"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
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
	isInitialized        bool
	mx                   sync.Mutex // mutex to protect micro dataset
}

// single global data set for all workers
var vmStats VmMicroData

// queue adapted from here: https://www.sobyte.net/post/2021-07/implementing-lock-free-queues-with-go/
// TODO: put it into new file 

// Unbounded lock-free for SmartContract Data
type RecordQueue struct {
	head unsafe.Pointer
	tail unsafe.Pointer
}

// Node
type RecordNode struct {
	value *SmartContractData
	next  unsafe.Pointer
}

// NewRecordQueue returns an empty queue.
func NewRecordQueue() *RecordQueue {
	n := unsafe.Pointer(&RecordNode{})
	return &RecordQueue{head: n, tail: n}
}

// Enqueue puts the given value v at the tail of the queue.
func (q *RecordQueue) Enqueue(v *SmartContractData) {
	n := &RecordNode{value: v}
	for {
		tail := load(&q.tail)
		next := load(&tail.next)
		if tail == load(&q.tail) { // are tail and next consistent?
			if next == nil {
				if cas(&tail.next, next, n) {
					cas(&q.tail, tail, n) // Enqueue is done.  try to swing tail to the inserted node
					return
				}
			} else { // tail was not pointing to the last node
				// try to swing Tail to the next node
				cas(&q.tail, tail, next)
			}
		}
	}
}

// Dequeue removes and returns the value at the head of the queue.
// It returns nil if the queue is empty.
func (q *RecordQueue) Dequeue() *SmartContractData {
	for {
		head := load(&q.head)
		tail := load(&q.tail)
		next := load(&head.next)
		if head == load(&q.head) { // are head, tail, and next consistent?
			if head == tail { // is queue empty or tail falling behind?
				if next == nil { // is queue empty?
					return nil
				}
				// tail is falling behind.  try to advance it
				cas(&q.tail, tail, next)
			} else {
				// read value before CAS otherwise another dequeue might free the next node
				v := next.value
				if cas(&q.head, head, next) {
					return v // Dequeue is done.  return
				}
			}
		}
	}
}

func load(p *unsafe.Pointer) (n *RecordNode) {
	return (*RecordNode)(atomic.LoadPointer(p))
}

func cas(p *unsafe.Pointer, old, new *RecordNode) (ok bool) {
	return atomic.CompareAndSwapPointer(
		p, unsafe.Pointer(old), unsafe.Pointer(new))
}

// Record Queue 
var recordQueue *RecordQueue = NewRecordQueue()

// Initialize summary data-set
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

var QueueSize uint64 = 0
const QueueMaxSize = 100000

// Flush the record queue and process records
func (d *VmMicroData) FlushProcessQueue() {
	// get access to dataset
	d.mx.Lock()

	// let only one process the queue (i.e. the first one in the lock observing 
	// that the queue is filled)
	for {
		// pop all records and process them
		scd := recordQueue.Dequeue()
		if scd != nil {
			// update opcode frequency
			for opCode, freq := range scd.OpCodeFrequency {
				value := d.opCodeFrequency[opCode]
				value.Add(&value, new(big.Int).SetUint64(uint64(freq)))
				d.opCodeFrequency[opCode] = value
			}

			// update instruction opCodeDuration
			for opCode, duration := range scd.OpCodeDuration {
				value := d.opCodeDuration[opCode]
				value.Add(&value, new(big.Int).SetUint64(uint64(duration)))
				d.opCodeDuration[opCode] = value
			}

			// update instruction frequency
			for instruction, freq := range scd.InstructionFrequency {
				value := d.instructionFrequency[instruction]
				value.Add(&value, new(big.Int).SetUint64(uint64(freq)))
				d.instructionFrequency[instruction] = value
			}

			// update basic block frequency
			for addr, bb := range scd.BasicBlockFrequency {
				bkey := BasicBlockKey{Contract: scd.Contract.String(), Address: addr, Instructions: hex.EncodeToString(bb.Instructions)}
				d.basicBlockFrequency[bkey] += bb.Frequency
			}

			// step length frequency
			value := d.stepLengthFrequency[scd.StepLength]
			value.Add(&value, new(big.Int).SetUint64(uint64(1)))
			d.stepLengthFrequency[scd.StepLength] = value

		} else {
			break
		}
	}
	// release data set
	d.mx.Unlock()
}

// update statistics
func (d *VmMicroData) UpdateStatistics(scd *SmartContractData) {

	// don't process record now; just put into the queue
	recordQueue.Enqueue(scd)
	atomic.AddUint64(&QueueSize, 1)

	// if threshold is reached; process all queued records
	if atomic.LoadUint64(&QueueSize) > QueueMaxSize {
		atomic.StoreUint64(&QueueSize, 0)
		d.FlushProcessQueue()
	}
}

// update statistics
func PrintStatistics() {
	// Flush queue
	vmStats.FlushProcessQueue()

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

	// switch synchronous mode off
	_, err = db.Exec("PRAGMA synchronous = OFF;PRAGMA journal_mode = MEMORY;")
	if err != nil {
		log.Fatalln(err.Error())
	}

	// begin Transaction
	const beginTransaction string = `BEGIN TRANSACTION;`
	const endTransaction string = `END TRANSACTION;`
	_, err = db.Exec(beginTransaction)
	if err != nil {
		log.Fatalln(err.Error())
	}

	// populate values
	insertFrequency := `INSERT INTO BasicBlockFrequency(contract, address, instructions, frequency) VALUES (?, ?, ?, ?)`
	statement, err := db.Prepare(insertFrequency)
	if err != nil {
		log.Fatalln(err.Error())
	}
	ctr := 1
	for bkey, freq := range vmStats.basicBlockFrequency {
		// commit dataset after 10000 entries
		if ctr >= 100000 {
			ctr = 1
			_, err = db.Exec(endTransaction)
			if err != nil {
				log.Fatalln(err.Error())
			}
			_, err = db.Exec(beginTransaction)
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
	_, err = db.Exec(endTransaction)
	if err != nil {
		log.Fatalln(err.Error())
	}

	// release data set
	vmStats.mx.Unlock()
}
