# START ALPHA CHANGE 3901 -- display help by default and retire submodule downloads --
.DEFAULT_GOAL := help
# FINISH ALPHA CHANGE 3901 -- display help by default and retire submodule downloads --

# START ALPHA CHANGE 3909 -- remove the unused Git executable check --
# FINISH ALPHA CHANGE 3909 -- remove the unused Git executable check --

.PHONY: check-go
check-go:
	@which go > /dev/null || (echo "Go is not installed.. Please install and try again."; exit 1)

.PHONY: check-protoc
check-protoc:
	@which protoc > /dev/null || (echo "protoc is not installed. Please install and try again."; exit 1)

# START ALPHA CHANGE 3904 -- retire the optional lint executable check --
# FINISH ALPHA CHANGE 3904 -- retire the optional lint executable check --

.PHONY: check-npm
check-npm:
	@which npm > /dev/null || (echo "npm is not installed. Please install and try again."; exit 1)

.PHONY: bindata
bindata: check-go
	go-bindata -pkg chain -o ./chain/chain_bindata.go ./chain/chains

# START ALPHA CHANGE 744 -- remove deleted legacy IBFT proto generation path --
.PHONY: protoc
protoc: check-protoc protoc-ibft
	protoc --go_out=. --go-grpc_out=. -I . -I=./validate --validate_out="lang=go:." \
	 ./server/proto/*.proto \
	 ./network/proto/*.proto \
	 ./syncer/proto/*.proto \
	 ./txpool/proto/*.proto \
	 ./consensus/innbcbft/proto/*.proto
# FINISH ALPHA CHANGE 744 -- remove deleted legacy IBFT proto generation path --

# START ALPHA CHANGE 1897 -- regenerate INNBC-owned IBFT wire types in place --
.PHONY: protoc-ibft
protoc-ibft: check-protoc
	protoc --go_out=paths=source_relative:. -I . \
	 ./consensus/innbcbft/ibftproto/messages.proto
# FINISH ALPHA CHANGE 1897 -- regenerate INNBC-owned IBFT wire types in place --

.PHONY: build
build: check-go
	$(eval VERSION ?= local)
	$(eval COMMIT_HASH ?= none)
	$(eval BRANCH ?= manual)
	$(eval TIME = $(shell date))
	go build -buildvcs=false -o innbc-chain -ldflags="\
		-X 'github.com/innovativebioresearch/innbc-chain/versioning.Version=$(VERSION)' \
		-X 'github.com/innovativebioresearch/innbc-chain/versioning.Commit=$(COMMIT_HASH)'\
		-X 'github.com/innovativebioresearch/innbc-chain/versioning.Branch=$(BRANCH)'\
		-X 'github.com/innovativebioresearch/innbc-chain/versioning.BuildTime=$(TIME)'" \
	main.go

# START ALPHA CHANGE 750 -- add production nodeclient bundle build target --
.PHONY: nodeclient
nodeclient: check-go
	bash ./scripts/build-nodeclient.sh
# FINISH ALPHA CHANGE 750 -- add production nodeclient bundle build target --

# START ALPHA CHANGE 764 -- add local nodeclient test workspace build target --
.PHONY: localtestclient
localtestclient: check-go
	bash ./scripts/build-localtestclient.sh
# FINISH ALPHA CHANGE 764 -- add local nodeclient test workspace build target --

# START ALPHA CHANGE 3905 -- retire the standalone lint target --
# FINISH ALPHA CHANGE 3905 -- retire the standalone lint target --

# START ALPHA CHANGE 3910 -- retire the dependency-license generator target --
# FINISH ALPHA CHANGE 3910 -- retire the dependency-license generator target --

.PHONY: test
test: check-go
	go test -race -shuffle=on -coverprofile coverage.out -timeout 20m `go list ./... | grep -v e2e`

.PHONY: fuzz-test
fuzz-test: check-go
	./scripts/fuzzAll

.PHONY: test-e2e
test-e2e: check-go
	go build -race -o artifacts/polygon-edge .
	env EDGE_BINARY=${PWD}/artifacts/polygon-edge go test -v -timeout=30m ./e2e/...

# START ALPHA CHANGE 3257 -- replace legacy INNBCBFT test targets with current workflow coverage --
.PHONY: test-current-workflow
test-current-workflow: check-go
	env INNBC_WORKFLOW_E2E=1 go test -race ./command/root \
		-run '^TestCurrentWorkflow_' -count=1 -timeout=45m
# FINISH ALPHA CHANGE 3257 -- replace legacy INNBCBFT test targets with current workflow coverage --

.PHONY: compile-core-contracts
compile-core-contracts: check-npm
	cd core-contracts && npm install && npm run compile
	$(MAKE) generate-smart-contract-bindings

.PHONY: generate-smart-contract-bindings
generate-smart-contract-bindings: check-go
	go run ./consensus/innbcbft/contractsapi/artifacts-gen/main.go
	go run ./consensus/innbcbft/contractsapi/bindings-gen/main.go

# START ALPHA CHANGE 1022 -- prune removed legacy Docker cluster helper targets --
# Legacy Polygon Edge Docker cluster helpers were removed because they depended on
# removed rootchain, bridge, StakeManager, SupernetManager, and burn-contract flows.
# FINISH ALPHA CHANGE 1022 -- prune removed legacy Docker cluster helper targets --

# START ALPHA CHANGE 3902 -- advertise explicit INNBC build commands --
.PHONY: help
help:
	@echo "Available targets (plain make displays this help):"
	@printf "  %-35s - %s\n" "help" "Display available commands (default)"
	@printf "  %-35s - %s\n" "bindata" "Generate Go binary data for chain"
	@printf "  %-35s - %s\n" "protoc" "Compile Protocol Buffers files"
	@printf "  %-35s - %s\n" "build" "Build the INNBC Chain binary"
# FINISH ALPHA CHANGE 3902 -- advertise explicit INNBC build commands --
	# START ALPHA CHANGE 1831 -- expose node package targets in Makefile help --
	@printf "  %-35s - %s\n" "nodeclient" "Build the production nodeclient bundle"
	@printf "  %-35s - %s\n" "localtestclient" "Build the local multi-node testing workspace"
	# FINISH ALPHA CHANGE 1831 -- expose node package targets in Makefile help --
# START ALPHA CHANGE 3906 -- remove the retired lint help entry --
# FINISH ALPHA CHANGE 3906 -- remove the retired lint help entry --
# START ALPHA CHANGE 3911 -- remove the retired license-generator help entry --
# FINISH ALPHA CHANGE 3911 -- remove the retired license-generator help entry --
	@printf "  %-35s - %s\n" "test" "Run unit tests"
	@printf "  %-35s - %s\n" "fuzz-test" "Run fuzz tests"
	@printf "  %-35s - %s\n" "test-e2e" "Run end-to-end tests"
	# START ALPHA CHANGE 3257 -- advertise the current workflow test target --
	@printf "  %-35s - %s\n" "test-current-workflow" "Run current node workflow integration tests with the race detector"
	# FINISH ALPHA CHANGE 3257 -- advertise the current workflow test target --
	@printf "  %-35s - %s\n" "compile-core-contracts" "Compile core contracts"
	@printf "  %-35s - %s\n" "generate-smart-contract-bindings" "Generate smart contract bindings"
	# START ALPHA CHANGE 1023 -- remove help entries for pruned Docker cluster helpers --
	# FINISH ALPHA CHANGE 1023 -- remove help entries for pruned Docker cluster helpers --
