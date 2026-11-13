module github.com/innovativebioresearch/innbc-chain

//START ALPHA CHANGE 1 -- update Go module directive for current tidy-compatible toolchain --
go 1.27.2

//FINISH ALPHA CHANGE 1 -- update Go module directive for current tidy-compatible toolchain --

require (
	// START ALPHA CHANGE 1880 -- replace external go-ibft with INNBC-owned packages --
	// FINISH ALPHA CHANGE 1880 -- replace external go-ibft with INNBC-owned packages --
	github.com/armon/go-metrics v0.4.1
	github.com/btcsuite/btcd v0.22.1
	github.com/coinbase/kryptology v1.8.0
	github.com/envoyproxy/protoc-gen-validate v1.3.3
	github.com/erigontech/mdbx-go v0.41.0 // ALPHA CHANGE 756 -- use MDBX as the direct storage backend dependency
	github.com/golang/protobuf v1.5.4
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.1
	github.com/hashicorp/go-hclog v1.6.2
	github.com/hashicorp/go-immutable-radix v1.3.1
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/golang-lru v1.0.2
	github.com/hashicorp/hcl v1.0.1-vault-5
	github.com/libp2p/go-libp2p v0.32.0
	github.com/libp2p/go-libp2p-kbucket v0.6.3
	github.com/libp2p/go-libp2p-pubsub v0.10.1
	github.com/mitchellh/mapstructure v1.5.0
	github.com/multiformats/go-multiaddr v0.12.3
	github.com/prometheus/client_golang v1.18.0
	github.com/quasilyte/go-ruleguard v0.4.0
	github.com/quasilyte/go-ruleguard/dsl v0.3.22
	github.com/ryanuber/columnize v2.1.2+incompatible
	github.com/sethvargo/go-retry v0.2.4
	github.com/spf13/cobra v1.8.0
	github.com/stretchr/testify v1.11.1
	github.com/trailofbits/go-fuzz-utils v0.0.0-20210901195358-9657fcfd256c
	github.com/umbracle/ethgo v0.1.4-0.20231006072852-6b068360fc97
	github.com/umbracle/fastrlp v0.1.1-0.20230504065717-58a1b8a9929d
	github.com/umbracle/go-eth-bn256 v0.0.0-20230125114011-47cb310d9b0b
	// START ALPHA CHANGE 1824 -- remove unused generic event tracker and final bbolt dependency --
	// FINISH ALPHA CHANGE 1824 -- remove unused generic event tracker and final bbolt dependency --
	golang.org/x/crypto v0.55.0
	golang.org/x/sync v0.22.0
	// START ALPHA CHANGE 2073 -- use x/sys directly for builder locks --
	golang.org/x/sys v0.47.0
	// ALPHA CHANGE 2223 -- use the existing token-bucket library directly.
	golang.org/x/time v0.14.0
	// START ALPHA CHANGE 1017 -- update x/tools and grpc/protobuf dependencies for current Go toolchain -- --
	golang.org/x/tools v0.48.0
	google.golang.org/grpc v1.83.2
	google.golang.org/protobuf v1.36.12
	gopkg.in/DataDog/dd-trace-go.v1 v1.63.1
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce
	// FINISH ALPHA CHANGE 1017 -- update x/tools and grpc/protobuf dependencies for current Go toolchain -- --
	gopkg.in/yaml.v3 v3.0.1
	pgregory.net/rapid v1.1.0
)

require (
	filippo.io/edwards25519 v1.0.0-rc.1 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/DataDog/appsec-internal-go v1.5.0 // indirect
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.48.0 // indirect
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state v0.48.1 // indirect
	github.com/DataDog/datadog-go/v5 v5.3.0 // indirect
	github.com/DataDog/go-libddwaf/v2 v2.4.2 // indirect
	github.com/DataDog/go-tuf v1.0.2-0.5.2 // indirect
	github.com/DataDog/gostackparse v0.7.0 // indirect
	github.com/DataDog/sketches-go v1.4.2 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	// START ALPHA CHANGE 1840 -- remove unused Gotty dependency --
	// FINISH ALPHA CHANGE 1840 -- remove unused Gotty dependency --
	github.com/andybalholm/brotli v1.0.6 // indirect
	github.com/benbjohnson/clock v1.3.5 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/btcsuite/btcd/chaincfg/chainhash v1.0.1 // indirect
	github.com/btcsuite/btcutil v1.0.3-0.20201208143702-a53e38424cce // indirect
	github.com/bwesterb/go-ristretto v1.2.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/consensys/gnark-crypto v0.5.3 // indirect
	github.com/containerd/cgroups v1.1.0 // indirect
	// START ALPHA CHANGE 1842 -- remove unused containerd continuity dependency --
	// FINISH ALPHA CHANGE 1842 -- remove unused containerd continuity dependency --
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/davidlazar/go-crypto v0.0.0-20200604182044-b73af7476f6c // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.2.0 // indirect
	github.com/docker/go-connections v0.5.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/ebitengine/purego v0.6.0-alpha.5 // indirect
	github.com/elastic/gosigar v0.14.2 // indirect
	github.com/fatih/color v1.15.0 // indirect
	github.com/flynn/noise v1.0.0 // indirect
	github.com/francoispqt/gojay v1.2.13 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/go-toolsmith/astcopy v1.0.2 // indirect
	github.com/go-toolsmith/astequal v1.0.3 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/gopacket v1.1.19 // indirect
	github.com/google/pprof v0.0.0-20231023181126-ff6d637d2a7b // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.5 // indirect
	github.com/huin/goupnp v1.3.0 // indirect
	github.com/ianlancetaylor/cgosymbolizer v0.0.0-20260504013507-f4b012c11129 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/ipfs/boxo v0.8.1 // indirect
	github.com/ipfs/go-cid v0.4.1 // indirect
	github.com/ipfs/go-log v1.0.5 // indirect
	github.com/ipfs/go-log/v2 v2.5.1 // indirect
	github.com/jackpal/go-nat-pmp v1.0.2 // indirect
	github.com/jbenet/go-temp-err-catcher v0.1.0 // indirect
	github.com/klauspost/compress v1.17.2 // indirect
	github.com/klauspost/cpuid/v2 v2.2.6 // indirect
	github.com/koron/go-ssdp v0.0.4 // indirect
	github.com/libp2p/go-buffer-pool v0.1.0 // indirect
	github.com/libp2p/go-cidranger v1.1.0 // indirect
	github.com/libp2p/go-flow-metrics v0.1.0 // indirect
	github.com/libp2p/go-libp2p-asn-util v0.3.0 // indirect
	github.com/libp2p/go-msgio v0.3.0 // indirect
	github.com/libp2p/go-nat v0.2.0 // indirect
	github.com/libp2p/go-netroute v0.2.1 // indirect
	github.com/libp2p/go-reuseport v0.4.0 // indirect
	github.com/libp2p/go-yamux/v4 v4.0.1 // indirect
	github.com/marten-seemann/tcp v0.0.0-20210406111302-dfbc87cc63fd // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/matttproud/golang_protobuf_extensions/v2 v2.0.0 // indirect
	github.com/miekg/dns v1.1.56 // indirect
	github.com/mikioh/tcpinfo v0.0.0-20190314235526-30a79bb1804b // indirect
	github.com/mikioh/tcpopt v0.0.0-20190314235656-172688c1accc // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/multiformats/go-base32 v0.1.0 // indirect
	github.com/multiformats/go-base36 v0.2.0 // indirect
	github.com/multiformats/go-multiaddr-dns v0.3.1 // indirect
	github.com/multiformats/go-multiaddr-fmt v0.1.0 // indirect
	github.com/multiformats/go-multibase v0.2.0 // indirect
	github.com/multiformats/go-multicodec v0.9.0 // indirect
	github.com/multiformats/go-multihash v0.2.3 // indirect
	github.com/multiformats/go-multistream v0.5.0 // indirect
	github.com/multiformats/go-varint v0.0.7 // indirect
	github.com/onsi/ginkgo/v2 v2.13.0 // indirect
	// START ALPHA CHANGE 1843 -- remove unused container and dockertest dependencies --
	// FINISH ALPHA CHANGE 1843 -- remove unused container and dockertest dependencies --
	github.com/opencontainers/runtime-spec v1.1.0 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/outcaste-io/ristretto v0.2.3 // indirect
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58 // indirect
	github.com/philhofer/fwd v1.1.2 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.45.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/quasilyte/gogrep v0.5.0 // indirect
	github.com/quasilyte/stdinfo v0.0.0-20220114132959-f7386bf02567 // indirect
	github.com/quic-go/qpack v0.4.0 // indirect
	github.com/quic-go/qtls-go1-20 v0.3.4 // indirect
	github.com/quic-go/quic-go v0.39.3 // indirect
	github.com/quic-go/webtransport-go v0.6.0 // indirect
	github.com/raulk/go-watchdog v1.3.0 // indirect
	github.com/richardartoul/molecule v1.0.1-0.20221107223329-32cfee06a052 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/secure-systems-lab/go-securesystemslib v0.7.0 // indirect
	// START ALPHA CHANGE 1844 -- remove unused logrus dependency --
	// FINISH ALPHA CHANGE 1844 -- remove unused logrus dependency --
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/tinylib/msgp v1.1.8 // indirect
	github.com/tyler-smith/go-bip39 v1.1.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.51.0 // indirect
	github.com/valyala/fastjson v1.6.3 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.17.1 // indirect
	go.uber.org/fx v1.20.1 // indirect
	go.uber.org/mock v0.3.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.26.0 // indirect
	golang.org/x/exp v0.0.0-20231006140011-7918f672742d // indirect
	golang.org/x/exp/typeparams v0.0.0-20230307190834-24139beb5833 // indirect
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	// FINISH ALPHA CHANGE 2073 -- use x/sys directly for builder locks --
	golang.org/x/telemetry v0.0.0-20260708182218-49f421fb7959 // indirect
	golang.org/x/text v0.41.0 // indirect
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	lukechampine.com/blake3 v1.2.1 // indirect
)
