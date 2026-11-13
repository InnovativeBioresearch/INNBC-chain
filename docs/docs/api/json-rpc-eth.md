## eth_chainId

Returns the currently configured chain id, a value used in replay-protected transaction signing as introduced by EIP-155.

### Parameters

* None

### Returns


* <b> QUANTITY </b> - big integer of the current chain id.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}'
````

<!-- START ALPHA CHANGE 3408 -- distinguish active bulk synchronization from proof of synchronization -->

## eth_syncing

Returns the currently tracked bulk-sync progression, when one exists.

### Parameters

* None

### Returns

* <b> Boolean (`false`) </b> - no bulk-sync progression is currently active.
* <b> Object </b> - the tracked bulk-sync progression:
  * <b>startingBlock: QUANTITY</b> - the first requested block of this bulk-sync operation.
  * <b>currentBlock: QUANTITY</b> - the latest block recorded by progression tracking.
  * <b>highestBlock: QUANTITY</b> - the target height advertised for this operation.
  * <b>type: String</b> - the progression type.

`false` does not establish that the node has reached the network's latest height. A node waiting without an eligible history-serving peer can have no active bulk-sync progression. Tracked progress is a snapshot and need not equal a separately queried current head.

<!-- FINISH ALPHA CHANGE 3408 -- distinguish active bulk synchronization from proof of synchronization -->

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_syncing","params":[],"id":1}'
````

## eth_getBlockByNumber

Returns block information by number.

### Parameters

*  <b>QUANTITY|TAG </b> - integer of a block number, or the string "latest"
*  <b> Boolean </b> - If true it returns the full transaction objects, if false only the hashes of the transactions.

### Returns

Object - A block object, or null when no block was found:

*  <b> number: QUANTITY </b> - the block number.
*  <b> hash: DATA, 32 Bytes </b> - hash of the block.
*  <b> parentHash: DATA, 32 Bytes </b> - hash of the parent block.
<!-- START ALPHA CHANGE 3394 -- describe INNBC block-number responses without proof-of-work assumptions -->
*  <b> nonce: DATA, 8 Bytes </b> - the header nonce field; it is not a proof-of-work solution.
*  <b> sha3Uncles: DATA, 32 Bytes </b> - SHA3 of the uncles data in the block.
*  <b> logsBloom: DATA, 256 Bytes </b>- the bloom filter for the logs of the block.
*  <b> transactionsRoot: DATA, 32 Bytes </b> - the root of the transaction trie of the block.
*  <b>stateRoot: DATA, 32 Bytes </b> - the root of the final state trie of the block.
*  <b> receiptsRoot: DATA, 32 Bytes </b> - the root of the receipts trie of the block.
*  <b> miner: DATA, 20 Bytes </b> - the header's miner field; it is not a statement of proof-of-work mining rewards.
*  <b> difficulty: QUANTITY </b> - the header difficulty field, not a proof-of-work fork-choice score.
*  <b> totalDifficulty: QUANTITY </b> - compatibility field currently populated from the header difficulty, not cumulative chain difficulty.
*  <b> extraData: DATA </b> - the “extra data” field of this block.
*  <b> size: QUANTITY </b> - integer the size of this block in bytes.
*  <b> gasLimit: QUANTITY </b> - the maximum gas allowed in this block.
*  <b> gasUsed: QUANTITY </b> - the total used gas by all transactions in this block.
*  <b> timestamp: QUANTITY </b> - the chain-clock timestamp. Each non-genesis value advances by the configured interval; it does not promise correspondence with elapsed real-world time.
<!-- FINISH ALPHA CHANGE 3394 -- describe INNBC block-number responses without proof-of-work assumptions -->
*  <b> transactions: Array </b> - Array of transaction objects, or 32 Bytes transaction hashes depending on the last given parameter.
*  <b> uncles: Array </b> - Array of uncle hashes.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true],"id":1}'
````

## eth_getBlockByHash

Returns block information by hash.

### Parameters

* <b> DATA , 32 Bytes </b> - Hash of a block.
* <b> Boolean </b> - If true it returns the full transaction objects, if false only the hashes of the transactions.

### Returns

<b> Object </b>  - A block object, or null when no block was found:

*  <b> number: QUANTITY </b> - the block number.
*  <b> hash: DATA, 32 Bytes </b> - hash of the block.
*  <b> parentHash: DATA, 32 Bytes </b> - hash of the parent block.
<!-- START ALPHA CHANGE 3394 -- describe INNBC block-hash responses without proof-of-work assumptions -->
*  <b> nonce: DATA, 8 Bytes </b> - the header nonce field; it is not a proof-of-work solution.
*  <b> sha3Uncles: DATA, 32 Bytes </b> - SHA3 of the uncles data in the block.
*  <b> logsBloom: DATA, 256 Bytes </b>- the bloom filter for the logs of the block.
*  <b> transactionsRoot: DATA, 32 Bytes </b> - the root of the transaction trie of the block.
*  <b>stateRoot: DATA, 32 Bytes </b> - the root of the final state trie of the block.
*  <b> receiptsRoot: DATA, 32 Bytes </b> - the root of the receipts trie of the block.
*  <b> miner: DATA, 20 Bytes </b> - the header's miner field; it is not a statement of proof-of-work mining rewards.
*  <b> difficulty: QUANTITY </b> - the header difficulty field, not a proof-of-work fork-choice score.
*  <b> totalDifficulty: QUANTITY </b> - compatibility field currently populated from the header difficulty, not cumulative chain difficulty.
*  <b> extraData: DATA </b> - the “extra data” field of this block.
*  <b> size: QUANTITY </b> - integer the size of this block in bytes.
*  <b> gasLimit: QUANTITY </b> - the maximum gas allowed in this block.
*  <b> gasUsed: QUANTITY </b> - the total used gas by all transactions in this block.
*  <b> timestamp: QUANTITY </b> - the chain-clock timestamp. Each non-genesis value advances by the configured interval; it does not promise correspondence with elapsed real-world time.
<!-- FINISH ALPHA CHANGE 3394 -- describe INNBC block-hash responses without proof-of-work assumptions -->
*  <b> transactions: Array </b> - Array of transaction objects, or 32 Bytes transaction hashes depending on the last given parameter.
*  <b> uncles: Array </b> - Array of uncle hashes.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_getBlockByHash","params":["0xdc0818cf78f21a8e70579cb46a43643f78291264dda342ae31049421c82d21ae",false],"id":1}'
````

## eth_blockNumber

Returns the number of the most recent block.

### Parameters

None

### Returns


*  <b> QUANTITY </b> - integer of the current block number the client is on.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
````

## eth_gasPrice

<!-- START ALPHA CHANGE 3394 -- describe the gas-price recommendation rather than an exact configured price -->
Returns a gas-price recommendation in wei.

With London active, the recommendation combines the base fee, floored at 1 wei, with the suggested priority fee, and applies the configured `--price-limit` as a lower bound. Addition saturates at the supported uint64 maximum rather than wrapping.

Before London, the recommendation uses the stored average gas price with the configured lower bound. Setting `--price-limit` does not force every recommendation to equal that value.
<!-- FINISH ALPHA CHANGE 3394 -- describe the gas-price recommendation rather than an exact configured price -->

---

### Parameters

None

### Returns


*  <b> QUANTITY </b> - integer of the current gas price in wei.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_gasPrice","params":[],"id":1}'
````

## eth_getBalance

Returns the balance of the account of the given address.

### Parameters

*  <b> DATA, 20 Bytes </b> - address to check for balance.
*  <b> QUANTITY|TAG </b> - integer block number, or the string "latest"

### Returns


*  <b> QUANTITY </b> - integer of the current balance in wei.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_getBalance","params":["0x407d73d8a49eeb85d32cf465507dd71d507100c1", "latest"],"id":1}'
````

## eth_sendRawTransaction

Creates new message call transaction or a contract creation for signed transactions.

### Parameters

*  <b> DATA </b> - The signed transaction data.

### Returns


*  <b> DATA, 32 Bytes </b> - <!-- START ALPHA CHANGE 3394 -- distinguish successful admission from RPC rejection -->the admitted transaction's hash. Decoding or admission failures return an RPC error rather than a successful zero-hash placeholder.<!-- FINISH ALPHA CHANGE 3394 -- distinguish successful admission from RPC rejection -->

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_sendRawTransaction","params":["0xd46e8dd67c5d32be8d46e8dd67c5d32be8058bb8eb970870f072445675058bb8eb970870f072445675"],"id":1}'
````

## eth_getTransactionByHash

Returns the information about a transaction requested by transaction hash.

### Parameters

*  <b> DATA, 32 Bytes </b> - hash of a transaction

### Returns

<b> Object </b> - A transaction object, or null when no transaction was found:

*  <b>  blockHash: DATA, 32 Bytes </b> - hash of the block where this transaction was in.
*  <b>  blockNumber: QUANTITY </b> - block number where this transaction was in.
*  <b>  from: DATA, 20 Bytes </b> - address of the sender.
*  <b>  gas: QUANTITY </b> - gas provided by the sender.
*  <b>  gasPrice: QUANTITY </b> - gas price provided by the sender in Wei.
*  <b>  hash: DATA, 32 Bytes </b> - hash of the transaction.
*  <b>  input: DATA </b> - the data send along with the transaction.
*  <b>  nonce: QUANTITY </b> - the number of transactions made by the sender prior to this one.
*  <b>  to: DATA, 20 Bytes </b> - address of the receiver. null when its a contract creation transaction.
*  <b>  transactionIndex: QUANTITY </b> - integer of the transactions index position in the block.
*  <b>  value: QUANTITY </b> - value transferred in Wei.
*  <b>  v: QUANTITY </b> - ECDSA recovery id
*  <b>  r: DATA, 32 Bytes </b> - ECDSA signature r
*  <b>  s: DATA, 32 Bytes </b> - ECDSA signature s

### Example
````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_getTransactionByHash","params":["0x88df016429689c079f3b2f6ad39fa052532c56795b733da78a91ebe6a713944b"],"id":1}'
````

## eth_getTransactionReceipt

Returns the receipt of a transaction by transaction hash.

Note That the receipt is not available for pending transactions.

### Parameters

*  <b> DATA, 32 Bytes </b> - hash of a transaction

### Returns

<b> Object </b>  - A transaction receipt object, or null when no receipt was found:

*  <b> transactionHash : DATA, 32 Bytes </b> - hash of the transaction.
*  <b> transactionIndex: QUANTITY </b> - integer of the transactions index position in the block.
*  <b> blockHash: DATA, 32 Bytes </b> - hash of the block where this transaction was in.
*  <b> blockNumber: QUANTITY </b> - block number where this transaction was in.
*  <b> from: DATA, 20 Bytes </b> - address of the sender.
*  <b> to: DATA, 20 Bytes </b> - address of the receiver. null when its a contract creation transaction.
*  <b> cumulativeGasUsed : QUANTITY </b> - The total amount of gas used when this transaction was executed in the block.
*  <b> gasUsed : QUANTITY </b> - The amount of gas used by this specific transaction alone.
*  <b> contractAddress : DATA, 20 Bytes </b> - The contract address created, if the transaction was a contract creation, otherwise null.
*  <b> logs: Array </b> - Array of log objects, which this transaction generated.
*  <b> logsBloom: DATA, 256 Bytes </b> - Bloom filter for light clients to quickly retrieve related logs.

It also returns either :

*  <b> root  : DATA 32 bytes </b> - post-transaction stateroot (pre Byzantium)
*  <b>status: QUANTITY </b> - either 1 (success) or 0 (failure)

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_getTransactionReceipt","params":["0xb903239f8543d04b5dc1ba6579132b143087c68db1b2168786408fcbce568238"],"id":1}'
````

## eth_getTransactionCount

Returns the number of transactions sent from an address.

### Parameters

*  <b>  DATA, 20 Bytes </b> - address.
*  <b>  QUANTITY|TAG </b> - integer block number, or the string "latest"

### Returns


*  <b>  QUANTITY </b> - integer of the number of transactions send from this address.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_getTransactionCount","params":["0x407d73d8a49eeb85d32cf465507dd71d507100c1","latest"],"id":1}'
````

## eth_getBlockTransactionCountByNumber

Returns the number of transactions in a block matching the given block number.

### Parameters

*  <b>  QUANTITY|TAG </b> - integer of a block number, or the string "latest"

### Returns


*  <b>  QUANTITY </b> - integer of the number of transactions in this block.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_getBlockTransactionCountByNumber","params":["latest"],"id":1}'
````

## eth_getLogs

Returns an array of all logs matching a given filter object.

### Parameters
<b> Object </b>  - The filter options:

*  <b> fromBlock: QUANTITY|TAG </b> - (optional, default: "latest") Integer block number, or "latest" for the last mined block
*  <b> toBlock: QUANTITY|TAG </b> - (optional, default: "latest") Integer block number, or "latest" for the last mined block
*  <b> address: DATA|Array, 20 Bytes </b> - (optional) Contract address or a list of addresses from which logs should originate.
*  <b> topics: Array of DATA </b> - (optional) Array of 32 Bytes DATA topics. Topics are order-dependent. Each topic can also be an array of DATA with “or” options.
<!-- START ALPHA CHANGE 3394 -- document implemented block-hash log queries and correct their return type -->
*  <b> blockHash: DATA, 32 Bytes </b> - optional block hash selecting a single-block lookup instead of the numeric range. Supply this option without fromBlock and toBlock.

### Returns

* <b> Array </b> - matching log objects, or an empty array when the successfully examined selection has no matching logs.

Unavailable retained history and unexpected missing blocks produce errors rather than successful partial log results. A numeric range extending beyond the current head stops at the available head.
<!-- FINISH ALPHA CHANGE 3394 -- document implemented block-hash log queries and correct their return type -->

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_getLogs","params":[{"topics": ["0x000000000000000000000000a94f5374fce5edbc8e2a8697c15331677e6ebf0b"]}],"id":1}'
````

## eth_getCode

Returns code at a given address.

### Parameters

*  <b>  DATA, 20 Bytes </b> - address
*  <b>  QUANTITY|TAG </b> - integer block number, or the string "latest"

### Returns


*  <b>  DATA </b> - the code from the given address.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_getCode","params":["0xa94f5374fce5edbc8e2a8697c15331677e6ebf0b", "0x2"],"id":1}'
````

## eth_call

Executes a new message call immediately without creating a transaction on the blockchain.

### Parameters
<b> Object </b>  - The transaction call object

*  <b>  from: DATA, 20 Bytes </b> - (optional) The address the transaction is sent from.
*  <b>  to: DATA, 20 Bytes </b> - The address the transaction is directed to.
*  <b>  gas: QUANTITY </b> - (optional) Integer of the gas provided for the transaction execution. eth_call consumes zero gas, but this parameter may be needed by some executions.
*  <b>  gasPrice: QUANTITY </b> - (optional) Integer of the gasPrice used for each paid gas
*  <b>  value: QUANTITY </b> - (optional) Integer of the value sent with this transaction
*  <b>  data: DATA </b> - (optional) Hash of the method signature and encoded parameters. For details see Ethereum Contract ABI in the Solidity documentation
*  <b>  QUANTITY|TAG </b> - integer block number, or the string "latest", see the default block paramete

### Returns


*  <b>  DATA </b> - the return value of executed contract.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_call","params":[{see above}],"id":1}'
````

## eth_getStorageAt

Returns the value from a storage position at a given address.

### Parameters

*  <b>  DATA, 20 Bytes </b> - address of the storage.
*  <b>  QUANTITY </b> - integer of the position in the storage.
*  <b>  QUANTITY|TAG </b> - integer block number, or the string "latest"

### Returns


*  <b>  DATA </b> - the value at this storage position.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_getStorageAt","params":["0x295a70b2de5e3953354a6a8344e616ed314d7251", "0x0", "latest"],"id":1}'
````

## eth_estimateGas

Generates and returns an estimate of how much gas is necessary to allow the transaction to complete. The transaction will not be added to the blockchain. Note that the estimate may be significantly more than the amount of gas actually used by the transaction, for a variety of reasons including EVM mechanics and node performance.

### Parameters

Expect that all properties are optional.

<b> Object </b>  - The transaction call object

*  <b>  from: DATA, 20 Bytes </b>  - The address the transaction is sent from.
*  <b>  to: DATA, 20 Bytes </b>  - The address the transaction is directed to.
*  <b>  gas: QUANTITY </b>  - Integer of the gas provided for the transaction execution. eth_call consumes zero gas, but this parameter may be needed by some executions.
*  <b>  gasPrice: QUANTITY </b>  - Integer of the gasPrice used for each paid gas
*  <b>  value: QUANTITY </b>  - Integer of the value sent with this transaction
*  <b>  data: DATA </b>  - Hash of the method signature and encoded parameters. For details see Ethereum Contract ABI in the Solidity documentation
*  <b>  QUANTITY|TAG </b>  - integer block number, or the string "latest", see the default block paramete

### Returns


*  <b>  QUANTITY </b> - the amount of gas used.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_estimateGas","params":[{see above}],"id":1}'
````

## eth_newFilter

Creates a filter object, based on filter options.
To get all matching logs for specific filter, call eth_getFilterLogs.
To check if the state has changed, call eth_getFilterChanges.

### Parameters
<b> Object </b> - The filter options:

*  <b>  fromBlock: QUANTITY|TAG </b> - (optional, default: "latest") Integer block number, or "latest" for the last mined block
*  <b>  toBlock: QUANTITY|TAG </b> - (optional, default: "latest") Integer block number, or "latest" for the last mined block
*  <b>  address: DATA|Array, 20 Bytes </b> - (optional) Contract address or a list of addresses from which logs should originate.
*  <b>  topics: Array of DATA </b> - (optional) Array of 32 Bytes DATA topics. Topics are order-dependent. Each topic can also be an array of DATA with “or” options.

### Returns


*  <b> QUANTITY </b> - A filter id.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_newFilter","params":[{"topics":["0x12341234"]}],"id":1}'
````

## eth_newBlockFilter

Creates a filter in the node, to notify when a new block arrives.
To check if the state has changed, call eth_getFilterChanges.

### Parameters

None

### Returns


1. QUANTITY - A filter id.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_newBlockFilter","params":[],"id":1}'
````

<!-- START ALPHA CHANGE 3408 -- replace the obsolete React terminal component in MkDocs -->
Use the curl example above to create the block filter.
<!-- FINISH ALPHA CHANGE 3408 -- replace the obsolete React terminal component in MkDocs -->

## eth_getFilterLogs

Returns an array of all logs matching filter with given id.

<!-- START ALPHA CHANGE 3408 -- use MkDocs admonition syntax and qualify historical query equivalence -->
!!! note "eth_getLogs and eth_getFilterLogs"

    `eth_getLogs(options)` and `eth_getFilterLogs(filterId)` use the same historical query path when the filter stores those options. Compare results against the same available chain view; selectors such as `latest` can resolve differently after the head advances.

    Both paths can return an error when the requested history is unavailable.
<!-- FINISH ALPHA CHANGE 3408 -- use MkDocs admonition syntax and qualify historical query equivalence -->

### Parameters

*  <b>  QUANTITY </b> - the filter id.

### Returns

<!-- START ALPHA CHANGE 3408 -- describe historical filter logs using current canonical-block metadata -->
<b>Array</b> - matching canonical-block log objects, or an empty array when the successfully examined selection has no matches.

| Field | Type | Meaning |
| --- | --- | --- |
| `removed` | Boolean | Currently `false`; this path does not produce reorganization-removal notifications. |
| `logIndex` | QUANTITY | Log index within the block. |
| `transactionIndex` | QUANTITY | Containing transaction's index within the block. |
| `transactionHash` | DATA, 32 bytes | Containing transaction hash. |
| `blockHash` | DATA, 32 bytes | Containing block hash. |
| `blockNumber` | QUANTITY | Containing block number. |
| `address` | DATA, 20 bytes | Address that emitted the log. |
| `data` | DATA | Encoded non-indexed log data. |
| `topics` | Array of DATA | Indexed topics; a non-anonymous Solidity event normally includes its event-signature hash as the first topic. |

This method does not return pending logs with null block metadata. Unavailable history produces an RPC error rather than an authoritative empty or partial result.
<!-- FINISH ALPHA CHANGE 3408 -- describe historical filter logs using current canonical-block metadata -->

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_getFilterLogs","params":["0x16"],"id":1}'
````

## eth_getFilterChanges

Polling method for a filter, which returns an array of logs that occurred since the last poll.

### Parameters

*  <b>  QUANTITY </b> - the filter id.

### Returns

<!-- START ALPHA CHANGE 3408 -- describe current block-filter and log-filter change payloads -->
<b>Array</b> - changes accumulated by the selected filter, or an empty array when no changes are available.

For `eth_newBlockFilter`, entries are block hashes.

For `eth_newFilter`, entries are logs from newly processed canonical blocks:

| Field | Type | Meaning |
| --- | --- | --- |
| `removed` | Boolean | Currently `false`; this path does not produce reorganization-removal notifications. |
| `logIndex` | QUANTITY | Log index within the block. |
| `transactionIndex` | QUANTITY | Containing transaction's index within the block. |
| `transactionHash` | DATA, 32 bytes | Containing transaction hash. |
| `blockHash` | DATA, 32 bytes | Containing block hash. |
| `blockNumber` | QUANTITY | Containing block number. |
| `address` | DATA, 20 bytes | Address that emitted the log. |
| `data` | DATA | Encoded non-indexed log data. |
| `topics` | Array of DATA | Indexed topics; a non-anonymous Solidity event normally includes its event-signature hash as the first topic. |

Log-filter changes do not contain pending logs with null block metadata.
<!-- FINISH ALPHA CHANGE 3408 -- describe current block-filter and log-filter change payloads -->

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_getFilterChanges","params":["0x16"],"id":1}'
````

## eth_uninstallFilter

Uninstalls a filter with a given id. Should always be called when a watch is no longer needed.
Additionally, filters timeout when they aren’t requested with eth_getFilterChanges for some time.

### Parameters

*  <b> QUANTITY </b> - The filter id.

### Returns


*  <b> Boolean </b> - true if the filter was successfully uninstalled, otherwise false.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_uninstallFilter","params":["0xb"],"id":1}'
````

## eth_unsubscribe

Subscriptions are cancelled with a regular RPC call with eth_unsubscribe as a method and the subscription id as the first parameter. It returns a bool indicating if the subscription was cancelled successfully.

### Parameters

*  <b> SUBSCRIPTION ID </b>

### Returns


*  <b>UNSUBSCRIBED FLAG </b> - true if the subscription was cancelled successful.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_unsubscribe","params":["0x9cef478923ff08bf67fde6c64013158d"],"id":1}'
````
