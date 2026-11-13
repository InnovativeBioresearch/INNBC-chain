
<!-- START ALPHA CHANGE 3403 -- qualify EVM compatibility by implemented opcode and fork support -->
The EVM executes smart contracts on INNBC Chain using the opcodes and fork rules implemented by this client.

Ethereum compatibility does not imply support for every instruction introduced by later Ethereum upgrades. For example, the current instruction table does not register `PUSH0`; executing an unsupported instruction returns an opcode-not-found error.

Compile contracts for an EVM target supported by this implementation and verify the resulting bytecode against the intended INNBC configuration. The main repository README remains the primary reference for INNBC-specific execution behavior.
<!-- FINISH ALPHA CHANGE 3403 -- qualify EVM compatibility by implemented opcode and fork support -->

!!! info "Overview of the EVM"

    Smart contracts represent self-executing agreements, with their terms explicitly encoded into the contract's code. Deployed on the Ethereum network, these contracts are processed by the EVM, which provides a secure, decentralized environment for execution. The EVM guarantees that all network nodes adhere to the same rules and yield identical outcomes when running smart contracts.

    Featuring a stack-based architecture, the EVM processes low-level instructions known as opcodes. Each opcode serves a specific function, such as performing arithmetic operations, managing storage access, or facilitating interactions with other contracts. To limit resource consumption during contract execution and prevent issues like infinite loops, the EVM employs gas as a metric for computational work.

    Developers typically create smart contracts using high-level programming languages, such as Solidity, which are subsequently compiled into EVM bytecode. The EVM executes this bytecode, ensuring the contract's logic is implemented as intended.
