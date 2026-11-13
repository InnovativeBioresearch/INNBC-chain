/* START ALPHA CHANGE 3076 -- verify snapshot CLI defaults and explicit destination overrides -- */
package snapshot

import (
	"testing"

	serverconfig "github.com/innovativebioresearch/innbc-chain/command/server/config"
	"github.com/innovativebioresearch/innbc-chain/command/snapshot/archive"
	"github.com/innovativebioresearch/innbc-chain/command/snapshot/backup"
	"github.com/innovativebioresearch/innbc-chain/command/snapshot/build"
	"github.com/innovativebioresearch/innbc-chain/command/snapshot/recoverycheckpoint"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestMDBXMapSize_SnapshotCommandDefaultsAndOverrides(t *testing.T) {
	require.Equal(t, uint64(1024), serverconfig.DefaultMDBXMapSizeGB)
	require.Equal(t, uint64(1024), serverconfig.DefaultConfig().History.MDBXMapSizeGB)

	for _, test := range []struct {
		name string
		new  func() *cobra.Command
	}{
		{name: "build", new: build.GetCommand},
		{name: "backup", new: backup.GetCommand},
		{name: "archive", new: archive.GetCommand},
		{name: "recovery_checkpoint", new: recoverycheckpoint.GetCommand},
	} {
		t.Run(test.name, func(t *testing.T) {
			cmd := test.new()
			value, err := cmd.Flags().GetUint64("mdbx-map-size-gb")
			require.NoError(t, err)
			require.Equal(t, uint64(1024), value)
			require.False(t, cmd.Flags().Changed("mdbx-map-size-gb"))

			require.NoError(t, cmd.ParseFlags([]string{"--mdbx-map-size-gb", "128"}))
			value, err = cmd.Flags().GetUint64("mdbx-map-size-gb")
			require.NoError(t, err)
			require.Equal(t, uint64(128), value)
			require.True(t, cmd.Flags().Changed("mdbx-map-size-gb"))

			require.NoError(t, cmd.ParseFlags([]string{"--mdbx-map-size-gb", "1024"}))
			value, err = cmd.Flags().GetUint64("mdbx-map-size-gb")
			require.NoError(t, err)
			require.Equal(t, uint64(1024), value)
		})
	}
}

/* FINISH ALPHA CHANGE 3076 -- verify snapshot CLI defaults and explicit destination overrides -- */
