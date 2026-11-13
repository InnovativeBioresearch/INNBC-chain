/* START ALPHA CHANGE 3270 -- verify archive RPC removal and retained synchronization reporting -- */
package server

import (
	"reflect"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/consensus"
	"github.com/innovativebioresearch/innbc-chain/helper/progress"
	"github.com/innovativebioresearch/innbc-chain/server/proto"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestLegacyArchiveRetired_OperatorRPCSurface(t *testing.T) {
	service := proto.File_server_proto_system_proto.Services().ByName("System")
	require.NotNil(t, service)

	for _, name := range []protoreflect.Name{"Export", "BlockByNumber"} {
		require.Nil(t, service.Methods().ByName(name))
		_, exists := reflect.TypeOf(&systemService{}).MethodByName(string(name))
		require.False(t, exists, "legacy implementation remains: %s", name)
	}

	for _, name := range []protoreflect.Name{
		"ExportRequest", "ExportEvent", "BlockByNumberRequest", "BlockResponse",
	} {
		require.Nil(t, proto.File_server_proto_system_proto.Messages().ByName(name))
	}

	var methods []string
	for i := 0; i < service.Methods().Len(); i++ {
		methods = append(methods, string(service.Methods().Get(i).Name()))
	}
	require.ElementsMatch(t, []string{
		"GetStatus",
		"PeersAdd",
		"PeersList",
		"PeersStatus",
		"Subscribe",
		"RefreshPeerDirectory",
	}, methods)

	var unaryMethods []string
	for _, method := range proto.System_ServiceDesc.Methods {
		unaryMethods = append(unaryMethods, method.MethodName)
	}
	require.ElementsMatch(t, []string{
		"GetStatus",
		"PeersAdd",
		"PeersList",
		"PeersStatus",
		"RefreshPeerDirectory",
	}, unaryMethods)

	var streams []string
	for _, stream := range proto.System_ServiceDesc.Streams {
		streams = append(streams, stream.StreamName)
	}
	require.Equal(t, []string{"Subscribe"}, streams)
}

type legacyArchiveRetiredConsensus struct {
	consensus.Consensus
	progression *progress.Progression
	calls       int
}

func (c *legacyArchiveRetiredConsensus) GetSyncProgression() *progress.Progression {
	c.calls++
	return c.progression
}

func TestLegacyArchiveRetired_ConsensusSyncProgression(t *testing.T) {
	tests := []struct {
		name        string
		progression *progress.Progression
	}{
		{name: "idle"},
		{
			name: "bulk_sync",
			progression: &progress.Progression{
				SyncType:      progress.ChainSyncBulk,
				StartingBlock: 10,
				CurrentBlock:  20,
				HighestBlock:  30,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			engine := &legacyArchiveRetiredConsensus{
				progression: tc.progression,
			}
			hub := &jsonRPCHub{Consensus: engine}

			got := hub.GetSyncProgression()
			require.Equal(t, 1, engine.calls)
			if tc.progression == nil {
				require.Nil(t, got)
			} else {
				require.Same(t, tc.progression, got)
			}
		})
	}
}

/* FINISH ALPHA CHANGE 3270 -- verify archive RPC removal and retained synchronization reporting -- */
