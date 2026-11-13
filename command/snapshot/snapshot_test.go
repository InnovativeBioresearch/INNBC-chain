/* START ALPHA CHANGE 2089 -- verify independent snapshot command registration -- */
package snapshot

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSnapshotCommandRegistration(t *testing.T) {
	command := GetCommand()

	for _, name := range []string{
		"build",
		"archive",
		"recovery-checkpoint",
	} {
		found, remaining, err := command.Find([]string{name})
		require.NoError(t, err)
		require.Empty(t, remaining)
		require.Equal(t, name, found.Name())
	}
}

/* FINISH ALPHA CHANGE 2089 -- verify independent snapshot command registration -- */
