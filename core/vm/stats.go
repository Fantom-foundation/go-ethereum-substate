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
	"time"
)

// Micro-Profiling data record for a single smart contract invocation
type MicroProfileData struct {
	OpCodeFrequency      map[OpCode]uint64        // opcode frequency stats
	OpCodeDuration       map[OpCode]time.Duration // opcode durations stats
	InstructionFrequency map[uint64]uint64        // instruction frequency stats
	StepLength           int                      // number of executed instructions
}

// Micro-profiling statistic
type MicroProfileStatistic struct {
	opCodeFrequency      map[OpCode]uint64 // opcode frequency statistics
	opCodeDuration       map[OpCode]uint64 // accumulated duration of opcodes
	instructionFrequency map[uint64]uint64 // instruction frequency statistics
	stepLengthFrequency  map[int]uint64    // smart contract length frequency
}

// Buffer size for micro-profiling channel
var MicroProfilingBufferSize int = 100000

// Buffer size for micro-profiling channel
var MicroProfilingDB string = "./microprofiling.db"

// Micro-Profiling channel
var mpChannel chan *MicroProfileData = make(chan *MicroProfileData, MicroProfilingBufferSize)

// Create new micro-profiling statistic
func NewMicroProfileStatistic() *MicroProfileStatistic {
	p := new(MicroProfileStatistic)
	p.opCodeFrequency = make(map[OpCode]uint64)
	p.opCodeDuration = make(map[OpCode]uint64)
	p.instructionFrequency = make(map[uint64]uint64)
	p.stepLengthFrequency = make(map[int]uint64)
	return p
}

// The data collector checks for a stopping signal and processes
// the workers' records via a channel. A data collector is a background task.
func MicroProfilingCollector(idx int, ctx context.Context, done chan struct{}, mps *MicroProfileStatistic) {
	defer close(done)
	for {
		select {

		// receive a new data record from a worker?
		case mpd := <- mpChannel:
			// process the data record and update the statistic

			// update op-code frequency
			for opCode, freq := range mpd.OpCodeFrequency {
				mps.opCodeFrequency[opCode] += freq
			}

			// update op-code duration
			for opCode, duration := range mpd.OpCodeDuration {
				mps.opCodeDuration[opCode] += uint64(duration)
			}

			// update instruction frequency
			for instructions, freq := range mpd.InstructionFrequency {
				mps.instructionFrequency[instructions] += freq
			}

			// step length frequency
			mps.stepLengthFrequency[mpd.StepLength]++

		// receive stop signal?
		case <-ctx.Done():
			if len(mpChannel) == 0 {
				return
			}
		}
	}
}

// put micro profiling data into the processing queue
func ProcessMicroProfileData(mpd *MicroProfileData) {
	mpChannel <- mpd
}

// Merge two micro-profiling statistics
func (mps *MicroProfileStatistic) Merge(src *MicroProfileStatistic) {
	// update opcode frequency
	for opCode, freq := range src.opCodeFrequency {
		mps.opCodeFrequency[opCode] += freq
	}

	// update instruction opCodeDuration
	for opCode, duration := range src.opCodeDuration {
		mps.opCodeDuration[opCode] += uint64(duration)
	}

	// update instruction frequency
	for instructions, freq := range src.instructionFrequency {
		mps.instructionFrequency[instructions] += freq
	}

	// step length frequency
	for length, freq := range src.stepLengthFrequency {
		mps.stepLengthFrequency[length] += freq
	}
}

// dump opcode frequency stats into a SQLITE3 database
func (mps *MicroProfileStatistic) dumpOpCodeFrequency(db *sql.DB) {
	// drop old frequency table and create new one
	_, err := db.Exec("DROP TABLE IF EXISTS OpCodeFrequency;CREATE TABLE OpCodeFrequency ( opcode TEXT NOT NULL, frequency INTEGER NOT NULL, PRIMARY KEY (opcode));")
	if err != nil {
		log.Fatalln(err.Error())
	}

	// prepare an insert statement for faster inserts and insert frequencies
	statement, err := db.Prepare("INSERT INTO OpCodeFrequency(opcode, frequency) VALUES (?, ?)")
	if err != nil {
		log.Fatalln(err.Error())
	}
	for opCode, freq := range mps.opCodeFrequency {
		_, err = statement.Exec(opCode, freq)
		if err != nil {
			log.Fatalln(err.Error())
		}

	}
}

// dump opcode duration statistic
func (mps *MicroProfileStatistic) dumpOpCodeDuration(db *sql.DB) {
	// drop old frequency table and create new one
	_, err := db.Exec("DROP TABLE IF EXISTS OpCodeDuration;CREATE TABLE OpCodeDuration ( opcode TEXT NOT NULL, duration NUMERIC NOT NULL, PRIMARY KEY (opcode));")
	if err != nil {
		log.Fatalln(err.Error())
	}

	// prepare an insert statement for faster inserts and insert frequencies
	statement, err := db.Prepare("INSERT INTO OpCodeDuration(opcode, duration) VALUES (?, ?)")
	if err != nil {
		log.Fatalln(err.Error())
	}
	for opCode, duration := range mps.opCodeDuration {
		_, err = statement.Exec(opCode, duration)
		if err != nil {
			log.Fatalln(err.Error())
		}

	}
}

// dump instruction frequency statistic
func (mps *MicroProfileStatistic) dumpInstructionFrequency(db *sql.DB) {
	// drop old frequency table and create new one
	_, err := db.Exec("DROP TABLE IF EXISTS InstructionFrequency;CREATE TABLE InstructionFrequency ( instructions INTEGER NOT NULL, frequency INTEGER NOT NULL, PRIMARY KEY (instructions));")
	if err != nil {
		log.Fatalln(err.Error())
	}

	// prepare an insert statement for faster inserts and insert frequencies
	statement, err := db.Prepare("INSERT INTO InstructionFrequency(instructions, frequency) VALUES (?, ?)")
	if err != nil {
		log.Fatalln(err.Error())
	}
	for instructions, freq := range mps.instructionFrequency {
		_, err = statement.Exec(instructions, freq)
		if err != nil {
			log.Fatalln(err.Error())
		}

	}
}

// dump step-length frequency statistic
func (mps *MicroProfileStatistic) dumpStepLengthFrequency(db *sql.DB) {
	// drop old frequency table and create new one
	_, err := db.Exec("DROP TABLE IF EXISTS StepLengthFrequency;CREATE TABLE StepLengthFrequency ( steplength INTEGER NOT NULL, frequency INTEGER NOT NULL, PRIMARY KEY (steplength));")
	if err != nil {
		log.Fatalln(err.Error())
	}

	// prepare an insert statement for faster inserts and insert frequencies
	statement, err := db.Prepare("INSERT INTO InstructionFrequency(steplength, frequency) VALUES (?, ?)")
	if err != nil {
		log.Fatalln(err.Error())
	}
	for length, freq := range mps.stepLengthFrequency {
		_, err = statement.Exec(length, freq)
		if err != nil {
			log.Fatalln(err.Error())
		}

	}
}

// dump micro-profiling statistic into a sqlite3 database
func (mps *MicroProfileStatistic) Dump(version string) {

	// open sqlite3 database
	// TODO: have parameters for sqlite3 database name
	db, err := sql.Open("sqlite3", MicroProfilingDB) // Open the created SQLite File
	if err != nil {
		log.Fatal(err.Error())
	}
	defer db.Close()

	// switch synchronous mode off, enable memory journaling,
	_, err = db.Exec("PRAGMA synchronous = OFF;PRAGMA journal_mode = MEMORY;")
	if err != nil {
		log.Fatalln(err.Error())
	}

	_, err = db.Exec("PRAGMA synchronous = OFF;PRAGMA journal_mode = MEMORY;")
	if err != nil {
		log.Fatalln(err.Error())
	}

	_, err = db.Exec("CREATE TABLE IF NOT EXISTS Information ( version TEXT );")
	if err != nil {
		log.Fatalln(err.Error())
	}

	_, err = db.Exec("INSERT INTO Information(version) VALUES (" + version + ")")
	if err != nil {
		log.Fatalln(err.Error())
	}

	// dump op-code frequencies
	mps.dumpOpCodeFrequency(db)

	// dump op-code durations
	mps.dumpOpCodeDuration(db)

	// dump instruction frequency
	mps.dumpInstructionFrequency(db)

	// dump step-length frequency
	mps.dumpStepLengthFrequency(db)
}
