## net_version

Returns the current network id.

### Parameters

None

### Returns


* <b> String </b> - The current network id.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"net_version","params":[],"id":83}'
````

<!-- START ALPHA CHANGE 3407 -- describe net_listening as the implemented compatibility response -->

## net_listening

The current implementation returns `true` as a compatibility response. It does not inspect listener state or test external reachability.

### Parameters

None

### Returns

* <b> Boolean </b> - `true`.

Do not use this response alone as proof of peer connectivity, listener health or synchronization.

<!-- FINISH ALPHA CHANGE 3407 -- describe net_listening as the implemented compatibility response -->

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"net_listening","params":[],"id":83}'
````

## net_peerCount

Returns number of peers currently connected to the client.

### Parameters

None

### Returns


* <b> QUANTITY </b> - integer of the number of connected peers.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"net_peerCount","params":[],"id":1}'
````
