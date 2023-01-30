package lfvm

import (
	"encoding/hex"
	"log"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/holiman/uint256"
)

// To run the benchmark use
//  go test ./core/vm/lfvm -bench=.*Fib.* --benchtime 10s

type example struct {
	code     []byte // Some contract code
	function uint32 // The identifier of the function in the contract to be called
}

func getContext() context {

	// Create a dummy contract
	addr := vm.AccountRef{}
	contract := vm.NewContract(addr, addr, big.NewInt(0), 1<<63)

	var exampleCode = []byte{
		byte(vm.PUSH4), 4}
	clearConversionCache()
	code, _ := Convert(common.Address{}, exampleCode, false, 0, false)
	data := make([]byte, 4+32)

	// Create execution context.
	return context{
		code:     code,
		data:     data,
		callsize: *uint256.NewInt(uint64(len(data))),
		stack:    NewStack(),
		memory:   NewMemory(),
		readOnly: true,
		contract: contract,
	}
}

// Test UseGas function and correct status after running out of gas
func TestGasFunc(t *testing.T) {
	ctx := getContext()
	ctx.contract.Gas = 100
	ok := ctx.UseGas(10)
	if !ok {
		t.Errorf("expected not failed useGas function, got failed")
	}
	if ctx.contract.Gas != 90 {
		t.Errorf("expected gas of contract in context is 90, got %d", ctx.contract.Gas)
	}
	ok = ctx.UseGas(100)
	if ok {
		t.Errorf("expected failed useGas function, got ok")
	}
	if ctx.contract.Gas != 90 {
		t.Errorf("expected gas of contract in context is 90 also after failing, got %d", ctx.contract.Gas)
	}
	if ctx.status != OUT_OF_GAS {
		t.Errorf("expected OUT_OF_GAS status 6, got %d", ctx.status)
	}
}

type OpcodeTest struct {
	name      string
	code      []Instruction
	fullStack bool
	argData   uint16
	endStatus Status
	isBerlin  bool
	isLondon  bool
}

func getInstructions(start OpCode, end OpCode) (opCodes []OpCode) {
	for i := start; i <= end; i++ {
		opCodes = append(opCodes, OpCode(i))
	}
	return
}

var fullStackFailOpCodes = []OpCode{
	MSIZE, ADDRESS, ORIGIN, CALLER, CALLVALUE, CALLDATASIZE,
	CODESIZE, GASPRICE, COINBASE, TIMESTAMP, NUMBER,
	DIFFICULTY, GASLIMIT, PC, GAS, RETURNDATASIZE,
	SELFBALANCE, CHAINID, BASEFEE,
	PUSH1_PUSH1_PUSH1_SHL_SUB,
	PUSH1_DUP1, PUSH1_PUSH1, PUSH1_PUSH4_DUP3,
}

var emptyStackFailOpCodes = []OpCode{
	POP, ADD, SUB, MUL, DIV, SDIV, MOD, SMOD, EXP, SIGNEXTEND,
	SHA3, LT, GT, SLT, SGT, EQ, AND, XOR, OR, BYTE,
	SHL, SHR, SAR, ADDMOD, MULMOD, ISZERO, NOT, BALANCE, CALLDATALOAD, EXTCODESIZE,
	BLOCKHASH, MLOAD, SLOAD, EXTCODEHASH, JUMP, SELFDESTRUCT,
	MSTORE, MSTORE8, SSTORE, JUMPI, RETURN, REVERT,
	CALLDATACOPY, CODECOPY, RETURNDATACOPY,
	EXTCODECOPY, CREATE, CREATE2, CALL, CALLCODE,
	STATICCALL, DELEGATECALL, POP_POP, POP_JUMP,
	SWAP2_POP, PUSH1_ADD, PUSH1_SHL, SWAP2_SWAP1_POP_JUMP,
	PUSH2_JUMPI, ISZERO_PUSH2_JUMPI, SWAP2_SWAP1,
	DUP2_LT, SWAP1_POP_SWAP2_SWAP1, POP_SWAP2_SWAP1_POP,
	AND_SWAP1_POP_SWAP2_SWAP1, SWAP1_POP, DUP2_MSTORE,
}

func addFullStackFailOpCodes(tests []OpcodeTest) []OpcodeTest {
	var addedTests []OpcodeTest
	addedTests = append(addedTests, tests...)
	var opCodes []OpCode
	opCodes = append(opCodes, fullStackFailOpCodes...)
	opCodes = append(opCodes, getInstructions(PUSH1, PUSH32)...)
	opCodes = append(opCodes, getInstructions(DUP1, DUP16)...)
	for _, opCode := range opCodes {
		addedTests = append(addedTests, OpcodeTest{opCode.String() + " Stack overflow", []Instruction{{opCode, 1}}, true, 0, ERROR, false, false})
	}
	return addedTests
}

func addEmptyStackFailOpCodes(tests []OpcodeTest) []OpcodeTest {
	var addedTests []OpcodeTest
	addedTests = append(addedTests, tests...)
	var opCodes []OpCode
	opCodes = append(opCodes, emptyStackFailOpCodes...)
	opCodes = append(opCodes, getInstructions(DUP1, DUP16)...)
	opCodes = append(opCodes, getInstructions(SWAP1, SWAP16)...)
	opCodes = append(opCodes, getInstructions(LOG0, LOG4)...)
	for _, opCode := range opCodes {
		addedTests = append(addedTests, OpcodeTest{opCode.String() + " Stack underflow", []Instruction{{opCode, 1}}, false, 0, ERROR, false, false})
	}
	return addedTests
}

func TestStackBoundry(t *testing.T) {

	// Create a dummy contract
	addr := vm.AccountRef{}
	contract := vm.NewContract(addr, addr, big.NewInt(0), 1<<63)

	// Add tests for execution
	tests := addFullStackFailOpCodes([]OpcodeTest{})
	tests = addEmptyStackFailOpCodes(tests)

	for _, test := range tests {

		// Create execution context.
		ctxt := context{
			code:     test.code,
			data:     nil,
			callsize: *uint256.NewInt(uint64(0)),
			stack:    NewStack(),
			memory:   NewMemory(),
			readOnly: true,
			contract: contract,
		}

		// Reset the context.
		ctxt.pc = 0
		ctxt.status = RUNNING
		ctxt.contract.Gas = 1 << 31
		if test.fullStack {
			ctxt.stack.stack_ptr = len(ctxt.stack.data)
		} else {
			ctxt.stack.stack_ptr = 0
		}

		// Run testing code
		run(&ctxt)

		// Check the result.
		if ctxt.status != test.endStatus {
			t.Errorf("execution failed %v: status is %v, wanted %v, error %v", test.name, ctxt.status, test.endStatus, ctxt.err)
		} else {
			t.Log("Success", test.name)
		}

	}
}

var opcodeTests = []OpcodeTest{
	{"POP OK", []Instruction{{PUSH1, 1}, {POP, 0}}, false, 0, STOPPED, false, false},
}

func addOKOpCodes(tests []OpcodeTest) []OpcodeTest {
	var addedTests []OpcodeTest
	addedTests = append(addedTests, tests...)
	for i := PUSH1; i <= PUSH32; i++ {
		code := []Instruction{{i, 1}}
		dataNum := int((i - PUSH1) / 2)
		for j := 0; j < dataNum; j++ {
			code = append(code, Instruction{DATA, 1})
		}
		addedTests = append(addedTests, OpcodeTest{i.String() + " execution", code, false, 0, STOPPED, false, false})
	}
	var opCodes []OpCode
	opCodes = append(opCodes, getInstructions(DUP1, SWAP16)...)
	for _, opCode := range opCodes {
		code := []Instruction{{opCode, 1}}
		addedTests = append(addedTests, OpcodeTest{opCode.String() + " execution", code, false, 0, STOPPED, false, false})
	}
	return addedTests
}

func TestOK(t *testing.T) {

	// Create a dummy contract
	addr := vm.AccountRef{}
	contract := vm.NewContract(addr, addr, big.NewInt(0), 1<<63)

	tests := addOKOpCodes(opcodeTests)

	for _, test := range tests {

		// Create execution context.
		ctxt := context{
			code:     test.code,
			data:     nil,
			callsize: *uint256.NewInt(uint64(0)),
			stack:    NewStack(),
			memory:   NewMemory(),
			readOnly: true,
			contract: contract,
		}

		// Reset the context.
		ctxt.pc = 0
		ctxt.status = RUNNING
		ctxt.contract.Gas = 1 << 31
		ctxt.stack.stack_ptr = 20

		// Run testing code
		run(&ctxt)

		// Check the result.
		if ctxt.status != test.endStatus {
			t.Errorf("execution failed %v: status is %v, wanted %v, error %v", test.name, ctxt.status, test.endStatus, ctxt.err)
		} else {
			t.Log("Success", test.name)
		}
	}
}

func getFibExample() example {
	// An implementation of the fib function in EVM byte code.
	code, err := hex.DecodeString("608060405234801561001057600080fd5b506004361061002b5760003560e01c8063f9b7c7e514610030575b600080fd5b61004a600480360381019061004591906100f6565b610060565b6040516100579190610132565b60405180910390f35b600060018263ffffffff161161007957600190506100b0565b61008e600283610089919061017c565b610060565b6100a360018461009e919061017c565b610060565b6100ad91906101b4565b90505b919050565b600080fd5b600063ffffffff82169050919050565b6100d3816100ba565b81146100de57600080fd5b50565b6000813590506100f0816100ca565b92915050565b60006020828403121561010c5761010b6100b5565b5b600061011a848285016100e1565b91505092915050565b61012c816100ba565b82525050565b60006020820190506101476000830184610123565b92915050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b6000610187826100ba565b9150610192836100ba565b9250828203905063ffffffff8111156101ae576101ad61014d565b5b92915050565b60006101bf826100ba565b91506101ca836100ba565b9250828201905063ffffffff8111156101e6576101e561014d565b5b9291505056fea26469706673582212207fd33e47e97ce5871bb05401e6710238af535ae8aeaab013ca9a9c29152b8a1b64736f6c637827302e382e31372d646576656c6f702e323032322e382e392b636f6d6d69742e62623161386466390058")
	if err != nil {
		log.Fatalf("Unable to decode fib-code: %v", err)
	}

	return example{
		code:     code,
		function: 0xF9B7C7E5, // The function selector for the fib function.
	}
}

func fib(x int) int {
	if x <= 1 {
		return 1
	}
	return fib(x-1) + fib(x-2)
}

func benchmarkFib(b *testing.B, arg int, with_super_instructions bool) {
	example := getFibExample()

	// Convert example to LFVM format.
	converted, err := convert(example.code, with_super_instructions)
	if err != nil {
		b.Fatalf("error converting code: %v", err)
	}

	// Create input data.

	// See details of argument encoding: t.ly/kBl6
	data := make([]byte, 4+32) // < the parameter is padded up to 32 bytes

	// Encode function selector in big-endian format.
	data[0] = byte(example.function >> 24)
	data[1] = byte(example.function >> 16)
	data[2] = byte(example.function >> 8)
	data[3] = byte(example.function)

	// Encode argument as a big-endian value.
	data[4+28] = byte(arg >> 24)
	data[5+28] = byte(arg >> 16)
	data[6+28] = byte(arg >> 8)
	data[7+28] = byte(arg)

	// Create a dummy contract
	addr := vm.AccountRef{}
	contract := vm.NewContract(addr, addr, big.NewInt(0), 1<<63)

	// Create execution context.
	ctxt := context{
		code:     converted,
		data:     data,
		callsize: *uint256.NewInt(uint64(len(data))),
		stack:    NewStack(),
		memory:   NewMemory(),
		readOnly: true,
		contract: contract,
	}

	// Compute expected value.
	wanted := fib(arg)

	for i := 0; i < b.N; i++ {
		// Reset the context.
		ctxt.pc = 0
		ctxt.status = RUNNING
		ctxt.contract.Gas = 1 << 31
		ctxt.stack.stack_ptr = 0

		// Run the code (actual benchmark).
		run(&ctxt)

		// Check the result.
		if ctxt.status != RETURNED {
			b.Fatalf("execution failed: status is %v, error %v", ctxt.status, ctxt.err)
		}

		size := ctxt.result_size
		if size.Uint64() != 32 {
			b.Fatalf("unexpected length of end; wanted 32, got %d", size.Uint64())
		}
		res := make([]byte, size.Uint64())
		offset := ctxt.result_offset
		ctxt.memory.CopyData(offset.Uint64(), res)

		got := (int(res[28]) << 24) | (int(res[29]) << 16) | (int(res[30]) << 8) | (int(res[31]) << 0)
		if wanted != got {
			b.Fatalf("unexpected result, wanted %d, got %d", wanted, got)
		}
	}
}

func BenchmarkFib10(b *testing.B) {
	benchmarkFib(b, 10, false)
}

func BenchmarkFib10_SI(b *testing.B) {
	benchmarkFib(b, 10, true)
}

var sink bool

func BenchmarkIsWriteInstruction(b *testing.B) {
	for i := 0; i < b.N; i++ {
		sink = isWriteInstruction(OpCode(i % int(NUM_EXECUTABLE_OPCODES)))
	}
}
