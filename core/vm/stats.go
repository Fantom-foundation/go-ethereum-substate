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
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"log"
	"math/big"
	"time"
)

// Micro-Profiling data record for a single smart contract invocation
type SmartContractData struct {
	OpCodeFrequency      map[OpCode]uint64        // opcode frequency stats
	OpCodeDuration       map[OpCode]time.Duration // opcode durations stats
	InstructionFrequency map[uint64]uint64        // instruction frequency stats
	StepLength           int                      // number of executed instructions
}

// Micro-profiling statistic for the VM
type VmMicroData struct {
	opCodeFrequency      map[OpCode]big.Int       // opcode frequency statistics
	opCodeDuration       map[OpCode]big.Int       // accumulated duration of opcodes
	instructionFrequency map[uint64]big.Int       // instruction frequency statistics
	stepLengthFrequency  map[int]big.Int          // smart contract length frequency
}

// Create new micro-profiling statistic
func NewVmMicroData() *VmMicroData {
	p := new(VmMicroData)
	p.opCodeFrequency = make(map[OpCode]big.Int)
	p.opCodeDuration = make(map[OpCode]big.Int)
	p.instructionFrequency = make(map[uint64]big.Int)
	p.stepLengthFrequency = make(map[int]big.Int)
	return p
}

// Channel for communication
// TODO: Buffer size as cli argument
const chSize = 100000
var ch chan *SmartContractData = make(chan *SmartContractData, chSize)

// The data collector checks for a stopping signal and processes
// the workers' records via a channel. A data collector is a background task.
func DataCollector(idx int, ctx context.Context, done chan struct{}, vmStats *VmMicroData) {
	defer close(done)
	for {
		select {

		// receive a new data record from a worker?
		case scd := <-ch:
			// process the data record and update the statistic

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

			// step length frequency
			value := vmStats.stepLengthFrequency[scd.StepLength]
			value.Add(&value, new(big.Int).SetUint64(uint64(1)))
			vmStats.stepLengthFrequency[scd.StepLength] = value

		// receive stop signal?
		case <-ctx.Done():
			if len(ch) == 0 {
				return
			}
		}
	}
}

// Put record into queue for later processing by collector worker
func ProcessSmartContractData(scd *SmartContractData) {
	ch <- scd
}

// Merge two micro-profiling statistics
func (vmStats *VmMicroData) merge(src *VmMicroData) {
	// update opcode frequency
	for opCode, freq := range src.opCodeFrequency {
		value := vmStats.opCodeFrequency[opCode]
		value.Add(&value, &freq)
		vmStats.opCodeFrequency[opCode] = value
	}

	// update instruction opCodeDuration
	for opCode, duration := range src.opCodeDuration {
		value := vmStats.opCodeDuration[opCode]
		value.Add(&value, &duration)
		vmStats.opCodeDuration[opCode] = value
	}

	// update instruction frequency
	for instruction, freq := range src.instructionFrequency {
		value := vmStats.instructionFrequency[instruction]
		value.Add(&value, &freq)
		vmStats.instructionFrequency[instruction] = value
	}

	// step length frequency
	for length, freq := range src.stepLengthFrequency {
		value := vmStats.stepLengthFrequency[length]
		value.Add(&value, &freq)
		vmStats.stepLengthFrequency[length] = value
	}

}

// dump opcode frequency stats into a SQLITE3 database
func (vmStats *VmMicroData) dumpOpCodeFrequency(db *sql.DB) {
	// drop old frequency table and create new one
	_, err := db.Exec("DROP TABLE IF EXISTS OpCodeFrequency;CREATE TABLE opcode_frequency ( opcode TEXT NOT NULL, freq INTEGER NOT NULL, PRIMARY KEY (opcode));")
	if err != nil {
		log.Fatalln(err.Error())
	}

	// prepare an insert statement for faster inserts and insert frequencies
	statement, err := db.Prepare("INSERT INTO opcode_frequency(opcode, frequency) VALUES (?, ?)")
	if err != nil {
		log.Fatalln(err.Error())
	}
	for opCode, freq := range vmStats.opCodeFrequency {
		_, err = statement.Exec(opCode, freq.String())
		if err != nil {
			log.Fatalln(err.Error())
		}

	}
}

// update statistics
func (vmStats *VmMicroData) dump() {

	// open sqlite3 database
	db, err := sql.Open("sqlite3", "./profiling.db") // Open the created SQLite File
	if err != nil {
		log.Fatal(err.Error())
	}
	defer db.Close()

	// switch synchronous mode off, enable memory journaling,
	_, err = db.Exec("PRAGMA synchronous = OFF;PRAGMA journal_mode = MEMORY;")
	if err != nil {
		log.Fatalln(err.Error())
	}

	// TODO: write version number and log record

	// Dump basic block frequency stats into a SQLITE database
	vmStats.dumpOpCodeFrequency(db)

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
}
