package lfvm

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/holiman/uint256"
)

type Stack struct {
	data      [1024]uint256.Int
	stack_ptr int
}

func (s *Stack) Data() []uint256.Int {
	return s.data[:s.stack_ptr]
}

func (s *Stack) push(d *uint256.Int) {
	s.data[s.stack_ptr] = *d
	s.stack_ptr++
}

func (s *Stack) pushEmpty() *uint256.Int {
	s.stack_ptr++
	return &s.data[s.stack_ptr-1]
}

func (s *Stack) pop() *uint256.Int {
	s.stack_ptr--
	return &s.data[s.stack_ptr]
}

func (s *Stack) len() int {
	return s.stack_ptr
}

func (s *Stack) swap(n int) {
	s.data[s.len()-n], s.data[s.len()-1] = s.data[s.len()-1], s.data[s.len()-n]
}

func (s *Stack) dup(n int) {
	s.data[s.stack_ptr] = s.data[s.stack_ptr-n]
	s.stack_ptr++
}

func (s *Stack) peek() *uint256.Int {
	return &s.data[s.len()-1]
}

func (s *Stack) Back(n int) *uint256.Int {
	return &s.data[s.len()-n-1]
}

func (s *Stack) full() bool {
	return s.stack_ptr >= len(s.data)
}

func ToHex(z *uint256.Int) string {
	var b bytes.Buffer
	b.WriteString("0x")
	bytes := z.Bytes32()
	for i, cur := range bytes {
		b.WriteString(fmt.Sprintf("%02x", cur))
		if (i+1)%8 == 0 {
			b.WriteString(" ")
		}
	}
	return b.String()
}

func (s *Stack) String() string {
	var b bytes.Buffer
	for i := 0; i < s.len(); i++ {
		b.WriteString(fmt.Sprintf("    [%2d] %v\n", s.len()-i-1, ToHex(s.Back(i))))
	}
	return b.String()
}

// ------------------ Stack Pool ------------------

var stackPool = sync.Pool{
	New: func() interface{} {
		return &Stack{}
	},
}

func NewStack() *Stack {
	return stackPool.Get().(*Stack)
}

func ReturnStack(s *Stack) {
	s.stack_ptr = 0
	stackPool.Put(s)
}
