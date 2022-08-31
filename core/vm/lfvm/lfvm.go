package lfvm

import "github.com/ethereum/go-ethereum/core/vm"

type EVMInterpreter struct {
	evm                     *vm.EVM
	cfg                     vm.Config
	with_super_instructions bool
	with_shadow_evm         bool
	with_statistics         bool
}

// Registers the long-form EVM as a possible interpreter implementation.
func init() {
	vm.RegisterInterpreterFactory("lfvm", func(evm *vm.EVM, cfg vm.Config) vm.EVMInterpreter {
		return &EVMInterpreter{evm: evm, cfg: cfg}
	})
	vm.RegisterInterpreterFactory("lfvm-si", func(evm *vm.EVM, cfg vm.Config) vm.EVMInterpreter {
		return &EVMInterpreter{evm: evm, cfg: cfg, with_super_instructions: true}
	})
	vm.RegisterInterpreterFactory("lfvm-dbg", func(evm *vm.EVM, cfg vm.Config) vm.EVMInterpreter {
		return &EVMInterpreter{evm: evm, cfg: cfg, with_shadow_evm: true}
	})
	vm.RegisterInterpreterFactory("lfvm-stats", func(evm *vm.EVM, cfg vm.Config) vm.EVMInterpreter {
		return &EVMInterpreter{evm: evm, cfg: cfg, with_statistics: true}
	})
	vm.RegisterInterpreterFactory("lfvm-si-stats", func(evm *vm.EVM, cfg vm.Config) vm.EVMInterpreter {
		return &EVMInterpreter{evm: evm, cfg: cfg, with_super_instructions: true, with_statistics: true}
	})
}

func (e *EVMInterpreter) Run(contract *vm.Contract, input []byte, readOnly bool) (ret []byte, err error) {
	converted, err := Convert(contract.Address(), contract.Code, e.with_super_instructions)
	if err != nil {
		panic(err)
		//return nil, err
	}
	return Run(e.evm, e.cfg, contract, converted, input, readOnly, e.evm.StateDB, e.with_shadow_evm, e.with_statistics)
}
