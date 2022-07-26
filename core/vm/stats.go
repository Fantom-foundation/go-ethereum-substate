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
	"fmt"
	"time"
	"math/big"
	"sync"
)

// VM Micro Dataset for profiling
type VmMicroData struct {
	opCodeFrequency      map[OpCode]big.Int // opcode frequency statistics
	opCodeDuration       map[OpCode]big.Int // accumulated duration of opcodes
	instructionFrequency map[uint64]big.Int // instruction frequency statistics
	stepLengthFrequency  map[int]big.Int // smart contract length frequency
	isInitialized        bool
	mx  sync.Mutex                          // mutex to protect micro dataset 
}

// single global data set for all workers
var vmStats VmMicroData

func (d *VmMicroData) Initialize() {
	d.mx.Lock()
	if (!d.isInitialized) {
		fmt.Println("initializing...")
		d.opCodeFrequency = make(map[OpCode]big.Int)
		d.opCodeDuration  = make(map[OpCode]big.Int)
		d.instructionFrequency = make(map[uint64]big.Int)
		d.stepLengthFrequency = make(map[int]big.Int)
		d.isInitialized = true
	}
	d.mx.Unlock()
}

// update statistics
func (d *VmMicroData) UpdateStatistics(opCodeFrequency map[OpCode]uint64, opCodeDuration map[OpCode]time.Duration, instructionFrequency map[uint64]uint64, stepLength int) {
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

	// step length frequency
	value := d.stepLengthFrequency[stepLength]
	value.Add(&value,new(big.Int).SetUint64(uint64(1)))

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
		fmt.Printf("opcode-total: %v,%v\n", opCodeToString[opCode], seconds.String())
	}

	// print instruction frequency
	for instruction, freq  := range vmStats.instructionFrequency {
		fmt.Printf("instruction-freq: %v,%v\n", instruction, freq.String())
	}

	// print step-length frequency
	for length, freq := range vmStats.stepLengthFrequency {
		fmt.Printf("steplen-freq: %v,%v\n", length, freq.String())
	}

	// release data set
	vmStats.mx.Unlock()
}
