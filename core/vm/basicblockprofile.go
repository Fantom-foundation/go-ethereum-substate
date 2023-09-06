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
	"log"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// BasicBlockProfiling flag controlled by cli.
var BasicBlockProfiling bool

// BasicBlockProfilingDB is the filename of the basic-block profiling db.
var BasicBlockProfilingDB string

// numRecords is the maximal number of records per SQLITE3 transaction for writing.
const numRecords = 1000

// BasicBlockProfilingBufferSize sets the buffer size for basic-block profiling.
var BasicBlockProfilingBufferSize int = 10000

// BasicBlockInfo contains runtime information of a basic block.
type BasicBlockInfo struct {
	Frequency uint64        // dynamic execution frequency of basic block
	Duration  time.Duration // accumulated runtime of basic block
}

// ContractInvocation contains basic-block infos for one invocation.
type ContractInvocation map[uint32]BasicBlockInfo

// BasicBlockKey uses code id and code address of basic block as a key.
type BasicBlockKey struct {
	CodeId  int    // code id of contract
	Address uint32 // code address (begin of a basic block)
}

// BasicBlockProfileStatistics for contracts/invocations.
type BasicBlockProfileStatistic map[BasicBlockKey]BasicBlockInfo

// NewBasicBlockProfileStatistics creates a new basic-block statistic
func NewBasicBlockProfileStatistic() BasicBlockProfileStatistic {
	return make(map[BasicBlockKey]BasicBlockInfo)
}

// Merge two basic-block profiling statistics.
func (bbps BasicBlockProfileStatistic) Merge(src BasicBlockProfileStatistic) {
	for key, info := range src {
		sinfo := bbps[key]
		sinfo.Frequency += info.Frequency
		sinfo.Duration += info.Duration
		bbps[key] = sinfo
	}
}

// BasiBlockProfileData record for a single smart contract invocation.
type BasicBlockProfileData struct {
	CodeId      int                // code id of contract
	ProfileInfo ContractInvocation // profiling information of an invocation
}

// bbpChannel is the basic-block profiling channel.
var bbpChannel chan *BasicBlockProfileData = make(chan *BasicBlockProfileData, BasicBlockProfilingBufferSize)

// ProcessBasicBlockProfileData puts a new record into profiling channel.
func ProcessBasicBlockProfileData(bbpd *BasicBlockProfileData) {
	bbpChannel <- bbpd
}

// BasicBlockProfilingCollector is the data collector that collects basic-block profiling
// data from evm invocations and updates the statistics.
func BasicBlockProfilingCollector(ctx context.Context, done chan struct{}, bbps BasicBlockProfileStatistic) {
	defer close(done)
	for {
		select {

		// receive a new data record from an evm instance
		case bbpd := <-bbpChannel:
			for addr, info := range bbpd.ProfileInfo {
				// construct new key for stats
				key := BasicBlockKey{CodeId: bbpd.CodeId, Address: addr}

				// update stats
				sinfo := bbps[key]
				sinfo.Frequency += info.Frequency
				sinfo.Duration += info.Duration
				bbps[key] = sinfo
			}

		// receive stop signal?
		case <-ctx.Done():
			if len(bbpChannel) == 0 {
				return
			}
		}
	}
}

// codeMutex for avoiding data races looking up code
var codeMutex sync.Mutex

// codeDictionary converts code (in hex format) to a unique ID for later lookup
var codeDictionary map[string]int = map[string]int{}

func CodeLookup(code string) int {
	codeMutex.Lock()
	var (
		id int
		ok bool
	)
	if id, ok = codeDictionary[code]; !ok {
		codeDictionary[code] = len(codeDictionary)
		id = len(codeDictionary) - 1
	}
	codeMutex.Unlock()
	return id
}

// Dump basic block frequency stats into a SQLITE3 database
func (bbps BasicBlockProfileStatistic) Dump() {
	// open sqlite3 database
	db, err := sql.Open("sqlite3", BasicBlockProfilingDB) // Open the created SQLite File
	if err != nil {
		log.Fatal(err.Error())
	}
	defer db.Close()

	// drop basic-block frequency table
	const dropBasicBlockFrequency string = `DROP TABLE IF EXISTS BasicBlockProfile;`
	_, err = db.Exec(dropBasicBlockFrequency)
	if err != nil {
		log.Fatalln(err.Error())
	}

	// create new table
	const createBasicBlockTable string = `
	CREATE TABLE BasicBlockProfile (
	 code_id INTEGER,
	 address INTEGER,
	 frequency INTEGER,
	 duration REAL
	);`
	_, err = db.Exec(createBasicBlockTable)
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
	insertFrequency := `INSERT INTO BasicBlockProfile(code_id, address, frequency, duration) VALUES (?, ?, ?, ?)`
	statement, err := db.Prepare(insertFrequency)
	if err != nil {
		log.Fatalln(err.Error())
	}

	// insert profile stats into the DB
	ctr := 1
	for key, info := range bbps {
		// commit dataset when record threshold is reached
		if ctr >= numRecords {
			ctr = 1
			_, err = db.Exec("END TRANSACTION; BEGIN TRANSACTION;")
			if err != nil {
				log.Fatalln(err.Error())
			}
		} else {
			ctr++
		}
		_, err = statement.Exec(key.CodeId, key.Address, info.Frequency, float64(info.Duration.Nanoseconds())/1e9)
		if err != nil {
			log.Fatalln(err.Error())
		}

	}

	// end transaction
	_, err = db.Exec("END TRANSACTION;")
	if err != nil {
		log.Fatalln(err.Error())
	}

	// drop code table
	const dropCode string = `DROP TABLE IF EXISTS Code;`
	_, err = db.Exec(dropCode)
	if err != nil {
		log.Fatalln(err.Error())
	}

	// create new table
	const createCode string = `
	CREATE TABLE Code (
	 code_id INTEGER PRIMARY KEY,
	 code TEXT
	);`
	_, err = db.Exec(createCode)
	if err != nil {
		log.Fatalln(err.Error())
	}

	// prepare the insert statement for faster inserts
	insertCode := `INSERT INTO Code(code_id, code) VALUES (?, ?)`
	statement, err = db.Prepare(insertCode)
	if err != nil {
		log.Fatalln(err.Error())
	}

	// dump code
	for code, id := range codeDictionary {
		_, err = statement.Exec(id, code)
		if err != nil {
			log.Fatalln(err.Error())
		}
	}
}
