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
	_ "github.com/mattn/go-sqlite3"
	"log"
	"encoding/hex"

	"github.com/ethereum/go-ethereum/common"
)

// Basic-block profiling flag controlled by cli
var BasicBlockProfiling bool

// Maximal number of records per SQLITE3 transaction for writing
const BasicBlockMaxNumRecords = 1000

// Buffer size for micro-profiling channel
var BasicBlockProfilingBufferSize int

// Name of SQLITE3 database
var BasicBlockProfilingDB string

// Basic-block data record for a single smart contract invocation
type BasicBlockProfileData struct {
	Contract            common.Address      // contract in hex format
	BasicBlockFrequency map[uint]BasicBlock // basic block frequency
}

// Basic-block data record for a single smart contract invocation
type BasicBlockKey struct {
	Contract     string // contract in hex format
	Instructions string // instructions in hex format
	Address      uint   // basic-block start address
}

// Basic-block statistic
type BasicBlockProfileStatistic struct {
	basicBlockFrequency map[BasicBlockKey]uint64 // basic block statistics
}

// Basic-Block Profiling channel
var bbpChannel chan *BasicBlockProfileData = make(chan *BasicBlockProfileData, BasicBlockProfilingBufferSize)

// Create new micro-profiling statistic
func NewBasicBlockProfileStatistic() *BasicBlockProfileStatistic {
	p := new(BasicBlockProfileStatistic)
	p.basicBlockFrequency = make(map[BasicBlockKey]uint64)
	return p
}

// The data collector checks for a stopping signal and processes
// the workers' records via a channel. A data collector is a background task.
func BasicBlockProfilingCollector(ctx context.Context, done chan struct{}, bbps *BasicBlockProfileStatistic) {
	defer close(done)
	for {
		select {

		// receive a new data record from a worker?
		case bbpd := <-bbpChannel:
			for addr, bb := range bbpd.BasicBlockFrequency {
				bkey := BasicBlockKey{Contract: bbpd.Contract.String(), Address: addr, Instructions: hex.EncodeToString(bb.Instructions)}
				bbps.basicBlockFrequency[bkey] += bb.Frequency
			}

		// receive stop signal?
		case <-ctx.Done():
			if len(bbpChannel) == 0 {
				return
			}
		}
	}
}

// put micro profiling data into the processing queue
func ProcessBasicBlockProfileData(bbpd *BasicBlockProfileData) {
	bbpChannel <- bbpd
}

// Merge two basic-block profiling statistics
func (bbps *BasicBlockProfileStatistic) Merge(src *BasicBlockProfileStatistic) {
	// update opcode frequency
	for bb, freq := range src.basicBlockFrequency {
		bbps.basicBlockFrequency[bb] += freq
	}
}

// dump basic block frequency stats into a SQLITE3 database
func (bbps *BasicBlockProfileStatistic) Dump() {
	// Dump basic-block frequency statistics into a SQLITE3 database

	// open sqlite3 database
	db, err := sql.Open("sqlite3", BasicBlockProfilingDB) // Open the created SQLite File
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
	for bkey, freq := range bbps.basicBlockFrequency {
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
