package lfvm

import (
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
)

type cache_key struct {
	addr            common.Address
	contract_length int
}

type cache_val struct {
	oldCode []byte
	code    Code
}

var changedAddress01 = common.HexToAddress("0xA7CC236F81b04c1058e9bfb70E0Ee9940e271676")
var changedAddress02 = common.HexToAddress("0xAD0FB83a110c3694faDa81e8B396716a610c4030")
var changedAddress03 = common.HexToAddress("0xA8B3C9f298877dD93F30E8Ed359956faE10E8797")

var mu = sync.Mutex{}
var cache = map[cache_key]cache_val{}

func clearConversionCache() {
	mu.Lock()
	defer mu.Unlock()
	cache = map[cache_key]cache_val{}
}

func Convert(addr common.Address, code []byte, with_super_instructions bool, blk uint64, create bool) (Code, error) {
	key := cache_key{addr, len(code)}
	mu.Lock()
	res, exists := cache[key]
	if exists && !create {
		isEqual := true
		if addr == changedAddress01 || addr == changedAddress02 || addr == changedAddress03 {
			// fmt.Println("Address: ", addr.String(), " blk: ", blk)

			for i, v := range res.oldCode {
				if v != code[i] {
					fmt.Println("Different code for address: ", addr.String(), " blk: ", blk)
					isEqual = false
					break
				}
			}
		}

		if isEqual {
			mu.Unlock()
			return res.code, nil
		}
	}
	mu.Unlock()
	resCode, error := convert(code, with_super_instructions)
	if error != nil {
		return nil, error
	}
	if !create {
		mu.Lock()
		cache[key] = cache_val{oldCode: code, code: resCode}
		mu.Unlock()
	}
	return resCode, nil
}

func convert(code []byte, with_super_instructions bool) (Code, error) {
	res := make([]Instruction, 0, len(code))

	// Convert each individual instruction.
	for i := 0; i < len(code); {
		// Handle jump destinations
		if code[i] == byte(vm.JUMPDEST) {
			if len(res) > i {
				return nil, fmt.Errorf("unable to convert code, encountered targe block larger than input")
			}
			// Jump to the next jump destination and fill space with noops
			if len(res) < i {
				res = append(res, Instruction{opcode: JUMP_TO, arg: uint16(i)})
			}
			for len(res) < i {
				res = append(res, Instruction{opcode: NOOP})
			}
			res = append(res, Instruction{opcode: JUMPDEST})
			i++
			continue
		}

		// Convert instructions
		instructions, inc, err := toInstructions(i, code, with_super_instructions)
		if err != nil {
			return nil, err
		}
		res = append(res, instructions...)
		i += inc + 1
	}
	return res, nil
}

func toInstructions(pos int, code []byte, with_super_instructions bool) ([]Instruction, int, error) {
	// Convert super instructions.
	if with_super_instructions {
		if len(code) > pos+7 {
			op0 := vm.OpCode(code[pos])
			op1 := vm.OpCode(code[pos+1])
			op2 := vm.OpCode(code[pos+2])
			op3 := vm.OpCode(code[pos+3])
			op4 := vm.OpCode(code[pos+4])
			op5 := vm.OpCode(code[pos+5])
			op6 := vm.OpCode(code[pos+6])
			op7 := vm.OpCode(code[pos+7])
			if op0 == vm.PUSH1 && op2 == vm.PUSH4 && op7 == vm.DUP3 {
				return []Instruction{
					{opcode: PUSH1_PUSH4_DUP3, arg: uint16(op1)},
					{opcode: DATA, arg: uint16(op3)<<8 | uint16(op4)},
					{opcode: DATA, arg: uint16(op5)<<8 | uint16(op6)},
				}, 7, nil
			}
			if op0 == vm.PUSH1 && op2 == vm.PUSH1 && op4 == vm.PUSH1 && op6 == vm.SHL && op7 == vm.SUB {
				return []Instruction{
					{opcode: PUSH1_PUSH1_PUSH1_SHL_SUB, arg: uint16(op1)<<8 | uint16(op3)},
					{opcode: DATA, arg: uint16(op5)},
				}, 7, nil
			}
		}
		if len(code) > pos+4 {
			op0 := vm.OpCode(code[pos])
			op1 := vm.OpCode(code[pos+1])
			op2 := vm.OpCode(code[pos+2])
			op3 := vm.OpCode(code[pos+3])
			op4 := vm.OpCode(code[pos+4])
			if op0 == vm.AND && op1 == vm.SWAP1 && op2 == vm.POP && op3 == vm.SWAP2 && op4 == vm.SWAP1 {
				return []Instruction{{opcode: AND_SWAP1_POP_SWAP2_SWAP1}}, 4, nil
			}
			if op0 == vm.ISZERO && op1 == vm.PUSH2 && op4 == vm.JUMPI {
				return []Instruction{{opcode: ISZERO_PUSH2_JUMPI, arg: uint16(op2)<<8 | uint16(op3)}}, 4, nil
			}
		}
		if len(code) > pos+3 {
			op0 := vm.OpCode(code[pos])
			op1 := vm.OpCode(code[pos+1])
			op2 := vm.OpCode(code[pos+2])
			op3 := vm.OpCode(code[pos+3])
			if op0 == vm.SWAP2 && op1 == vm.SWAP1 && op2 == vm.POP && op3 == vm.JUMP {
				return []Instruction{{opcode: SWAP2_SWAP1_POP_JUMP}}, 3, nil
			}
			if op0 == vm.SWAP1 && op1 == vm.POP && op2 == vm.SWAP2 && op3 == vm.SWAP1 {
				return []Instruction{{opcode: SWAP1_POP_SWAP2_SWAP1}}, 3, nil
			}
			if op0 == vm.POP && op1 == vm.SWAP2 && op2 == vm.SWAP1 && op3 == vm.POP {
				return []Instruction{{opcode: POP_SWAP2_SWAP1_POP}}, 3, nil
			}
			if op0 == vm.PUSH2 && op3 == vm.JUMP {
				return []Instruction{{opcode: PUSH2_JUMP, arg: uint16(op1)<<8 | uint16(op2)}}, 3, nil
			}
			if op0 == vm.PUSH2 && op3 == vm.JUMPI {
				return []Instruction{{opcode: PUSH2_JUMPI, arg: uint16(op1)<<8 | uint16(op2)}}, 3, nil
			}
			if op0 == vm.PUSH1 && op2 == vm.PUSH1 {
				return []Instruction{{opcode: PUSH1_PUSH1, arg: uint16(op1)<<8 | uint16(op3)}}, 3, nil
			}
		}
		if len(code) > pos+2 {
			op0 := vm.OpCode(code[pos])
			op1 := vm.OpCode(code[pos+1])
			op2 := vm.OpCode(code[pos+2])
			if op0 == vm.PUSH1 && op2 == vm.ADD {
				return []Instruction{{opcode: PUSH1_ADD, arg: uint16(op1)}}, 2, nil
			}
			if op0 == vm.PUSH1 && op2 == vm.SHL {
				return []Instruction{{opcode: PUSH1_SHL, arg: uint16(op1)}}, 2, nil
			}
			if op0 == vm.PUSH1 && op2 == vm.DUP1 {
				return []Instruction{{opcode: PUSH1_DUP1, arg: uint16(op1)}}, 2, nil
			}
		}
		if len(code) > pos+1 {
			op0 := vm.OpCode(code[pos])
			op1 := vm.OpCode(code[pos+1])
			if op0 == vm.SWAP1 && op1 == vm.POP {
				return []Instruction{{opcode: SWAP1_POP}}, 1, nil
			}
			if op0 == vm.POP && op1 == vm.JUMP {
				return []Instruction{{opcode: POP_JUMP}}, 1, nil
			}
			if op0 == vm.POP && op1 == vm.POP {
				return []Instruction{{opcode: POP_POP}}, 1, nil
			}
			if op0 == vm.SWAP2 && op1 == vm.SWAP1 {
				return []Instruction{{opcode: SWAP2_SWAP1}}, 1, nil
			}
			if op0 == vm.SWAP2 && op1 == vm.POP {
				return []Instruction{{opcode: SWAP2_POP}}, 1, nil
			}
			if op0 == vm.DUP2 && op1 == vm.MSTORE {
				return []Instruction{{opcode: DUP2_MSTORE}}, 1, nil
			}
			if op0 == vm.DUP2 && op1 == vm.LT {
				return []Instruction{{opcode: DUP2_LT}}, 1, nil
			}
		}
	}

	// Convert individual instructions.
	opcode := vm.OpCode(code[pos])

	if opcode == vm.PC {
		if pos > 1<<16 {
			panic("PC counter exceeding 16 bit limit")
		}
		return []Instruction{{opcode: PC, arg: uint16(pos)}}, 0, nil
	}

	if vm.PUSH1 <= opcode && opcode <= vm.PUSH32 {
		// Determine the number of bytes to be pushed.
		n := int(opcode) - int(vm.PUSH1) + 1

		// If there are not enough bytes left in the code, the instruction is invalid.
		// It is likely the case that we are in a data segment.
		if len(code) < pos+n+2 {
			return []Instruction{{opcode: INVALID}}, 1, nil
		}

		// Fix the op-codes of the resulting instructions
		res := make([]Instruction, n/2+n%2)
		for i := range res {
			res[i].opcode = DATA
			res[i].arg = 0
		}
		res[0].opcode = PUSH1 + OpCode(n-1)

		// Fix the arguments by packing them in pairs into the instructions.
		for i := 0; i < n; i += 2 {
			res[i/2].arg = uint16(code[pos+i+1])<<8 | uint16(code[pos+i+2])
		}
		if n%2 == 1 {
			res[n/2].arg = uint16(code[pos+n]) << 8
		}
		return res, n, nil
	}

	// All the rest converts to a single instruction.
	instruction, err := toInstruction(opcode)
	return []Instruction{instruction}, 0, err
}

var op_2_op = map[vm.OpCode]OpCode{
	// Stack operations
	vm.POP:  POP,
	vm.PUSH: INVALID,

	vm.DUP1:  DUP1,
	vm.DUP2:  DUP2,
	vm.DUP3:  DUP3,
	vm.DUP4:  DUP4,
	vm.DUP5:  DUP5,
	vm.DUP6:  DUP6,
	vm.DUP7:  DUP7,
	vm.DUP8:  DUP8,
	vm.DUP9:  DUP9,
	vm.DUP10: DUP10,
	vm.DUP11: DUP11,
	vm.DUP12: DUP12,
	vm.DUP13: DUP13,
	vm.DUP14: DUP14,
	vm.DUP15: DUP15,
	vm.DUP16: DUP16,

	vm.SWAP1:  SWAP1,
	vm.SWAP2:  SWAP2,
	vm.SWAP3:  SWAP3,
	vm.SWAP4:  SWAP4,
	vm.SWAP5:  SWAP5,
	vm.SWAP6:  SWAP6,
	vm.SWAP7:  SWAP7,
	vm.SWAP8:  SWAP8,
	vm.SWAP9:  SWAP9,
	vm.SWAP10: SWAP10,
	vm.SWAP11: SWAP11,
	vm.SWAP12: SWAP12,
	vm.SWAP13: SWAP13,
	vm.SWAP14: SWAP14,
	vm.SWAP15: SWAP15,
	vm.SWAP16: SWAP16,

	// Memory operations
	vm.MLOAD:   MLOAD,
	vm.MSTORE:  MSTORE,
	vm.MSTORE8: MSTORE8,
	vm.MSIZE:   MSIZE,

	// Storage operations
	vm.SLOAD:  SLOAD,
	vm.SSTORE: SSTORE,

	// Control flow
	vm.JUMP:    JUMP,
	vm.JUMPI:   JUMPI,
	vm.STOP:    STOP,
	vm.RETURN:  RETURN,
	vm.REVERT:  REVERT,
	vm.INVALID: INVALID,
	vm.PC:      PC,

	// Arithmethic operations
	vm.ADD:        ADD,
	vm.MUL:        MUL,
	vm.SUB:        SUB,
	vm.DIV:        DIV,
	vm.SDIV:       SDIV,
	vm.MOD:        MOD,
	vm.SMOD:       SMOD,
	vm.ADDMOD:     ADDMOD,
	vm.MULMOD:     MULMOD,
	vm.EXP:        EXP,
	vm.SIGNEXTEND: SIGNEXTEND,

	// Complex function
	vm.SHA3: SHA3,

	// Comparison operations
	vm.LT:     LT,
	vm.GT:     GT,
	vm.SLT:    SLT,
	vm.SGT:    SGT,
	vm.EQ:     EQ,
	vm.ISZERO: ISZERO,

	// Bit-pattern operations
	vm.AND:  AND,
	vm.OR:   OR,
	vm.XOR:  XOR,
	vm.NOT:  NOT,
	vm.BYTE: BYTE,
	vm.SHL:  SHL,
	vm.SHR:  SHR,
	vm.SAR:  SAR,

	// System instructions
	vm.ADDRESS:        ADDRESS,
	vm.BALANCE:        BALANCE,
	vm.ORIGIN:         ORIGIN,
	vm.CALLER:         CALLER,
	vm.CALLVALUE:      CALLVALUE,
	vm.CALLDATALOAD:   CALLDATALOAD,
	vm.CALLDATASIZE:   CALLDATASIZE,
	vm.CALLDATACOPY:   CALLDATACOPY,
	vm.CODESIZE:       CODESIZE,
	vm.CODECOPY:       CODECOPY,
	vm.GAS:            GAS,
	vm.GASPRICE:       GASPRICE,
	vm.EXTCODESIZE:    EXTCODESIZE,
	vm.EXTCODECOPY:    EXTCODECOPY,
	vm.RETURNDATASIZE: RETURNDATASIZE,
	vm.RETURNDATACOPY: RETURNDATACOPY,
	vm.EXTCODEHASH:    EXTCODEHASH,
	vm.CREATE:         CREATE,
	vm.CALL:           CALL,
	vm.CALLCODE:       CALLCODE,
	vm.DELEGATECALL:   DELEGATECALL,
	vm.CREATE2:        CREATE2,
	vm.STATICCALL:     STATICCALL,
	vm.SELFDESTRUCT:   SELFDESTRUCT,

	// Block chain instructions
	vm.BLOCKHASH:   BLOCKHASH,
	vm.COINBASE:    COINBASE,
	vm.TIMESTAMP:   TIMESTAMP,
	vm.NUMBER:      NUMBER,
	vm.DIFFICULTY:  DIFFICULTY,
	vm.GASLIMIT:    GASLIMIT,
	vm.CHAINID:     CHAINID,
	vm.SELFBALANCE: SELFBALANCE,
	vm.BASEFEE:     BASEFEE,

	// Log instructions
	vm.LOG0: LOG0,
	vm.LOG1: LOG1,
	vm.LOG2: LOG2,
	vm.LOG3: LOG3,
	vm.LOG4: LOG4,
}

func toInstruction(opcode vm.OpCode) (Instruction, error) {
	res, found := op_2_op[opcode]
	if !found {
		if !isValidVmOpcode(opcode) {
			// Everything that is not an op-code results in an invalid instruction.
			res = INVALID
		} else {
			return Instruction{}, fmt.Errorf("unsupported opcode in converter: %v", opcode)
		}
	}
	return Instruction{opcode: res}, nil
}

func isValidVmOpcode(code vm.OpCode) bool {
	return code != vm.SWAP && code != vm.DUP && code != vm.POP && fmt.Sprintf("%v", code)[0:2] != "op"
}
