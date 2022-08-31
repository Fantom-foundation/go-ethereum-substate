package lfvm

import "github.com/ethereum/go-ethereum/core/vm"

type EVMInterpreter struct {
	evm             *vm.EVM
	cfg             vm.Config
	with_shadow_evm bool
}

// Registers the long-form EVM as a possible interpreter implementation.
func init() {
	vm.RegisterInterpreterFactory("lfvm", func(evm *vm.EVM, cfg vm.Config) vm.EVMInterpreter {
		return &EVMInterpreter{evm: evm, cfg: cfg}
	})
	vm.RegisterInterpreterFactory("lfvm-dbg", func(evm *vm.EVM, cfg vm.Config) vm.EVMInterpreter {
		return &EVMInterpreter{evm: evm, cfg: cfg, with_shadow_evm: true}
	})
}

func (e *EVMInterpreter) Run(contract *vm.Contract, input []byte, readOnly bool) (ret []byte, err error) {
	converted, err := Convert(contract.Address(), contract.Code, false)
	if err != nil {
		panic(err)
		//return nil, err
	}
	return Run(e.evm, e.cfg, contract, converted, input, readOnly, e.evm.StateDB, e.with_shadow_evm, false)
}
