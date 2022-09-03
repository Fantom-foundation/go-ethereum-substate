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
	"sync/atomic"
	"unsafe"
)

// implements a lock-free queue for stats records
//
// https://www.cs.rochester.edu/u/scott/papers/1996_PODC_queues.pdf
// implementation is adapted from here: 
// https://www.sobyte.net/post/2021-07/implementing-lock-free-queues-with-go/
// 

// Unbounded lock-free for SmartContract Data
type RecordQueue struct {
	head unsafe.Pointer
	tail unsafe.Pointer
}

// Record Node
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

// atomic load a pointer
func load(p *unsafe.Pointer) (n *RecordNode) {
	return (*RecordNode)(atomic.LoadPointer(p))
}

// atomic compare and swap operation
func cas(p *unsafe.Pointer, old, new *RecordNode) (ok bool) {
	return atomic.CompareAndSwapPointer(
		p, unsafe.Pointer(old), unsafe.Pointer(new))
}
