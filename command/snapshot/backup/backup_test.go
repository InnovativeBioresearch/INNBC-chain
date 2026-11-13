/* START ALPHA CHANGE 2267 -- test backup CLI failure status in plain and JSON modes -- */
package backup

import (
	"bytes"
	"encoding/json"
	"errors"

	/* START ALPHA CHANGE 3283 -- report subprocess setup errors without manufacturing an expected failure status -- */
	"fmt"
	/* FINISH ALPHA CHANGE 3283 -- report subprocess setup errors without manufacturing an expected failure status -- */
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/command/helper"
	"github.com/stretchr/testify/require"
)

func TestDatabaseBackupCommandFailure(t *testing.T) {
	for _, mode := range []string{"plain", "json"} {
		t.Run(mode, func(t *testing.T) {
			process := exec.Command(
				os.Args[0],
				"-test.run=^TestDatabaseBackupCommandFailureHelper$",
			)
			process.Env = append(
				os.Environ(),
				"INNBC_DATABASE_BACKUP_TEST_HELPER=1",
				"INNBC_DATABASE_BACKUP_TEST_MODE="+mode,
				"INNBC_DATABASE_BACKUP_TEST_ROOT="+t.TempDir(),
			)

			var stdout, stderr bytes.Buffer
			process.Stdout = &stdout
			process.Stderr = &stderr

			err := process.Run()
			var exitError *exec.ExitError
			require.True(t, errors.As(err, &exitError))
			require.Equal(t, 1, exitError.ExitCode())
			require.Empty(t, strings.TrimSpace(stdout.String()))
			require.NotEmpty(t, strings.TrimSpace(stderr.String()))

			if mode == "json" {
				var response struct {
					Error string `json:"error"`
				}
				require.NoError(t, json.Unmarshal(stderr.Bytes(), &response))
				require.NotEmpty(t, response.Error)
			}
		})
	}
}

func TestDatabaseBackupCommandFailureHelper(t *testing.T) {
	if os.Getenv("INNBC_DATABASE_BACKUP_TEST_HELPER") != "1" {
		return
	}

	root := os.Getenv("INNBC_DATABASE_BACKUP_TEST_ROOT")
	cmd := GetCommand()
	helper.RegisterJSONOutputFlag(cmd)

	args := []string{
		"--chain", filepath.Join(root, "genesis.json"),
		"--data-dir", filepath.Join(root, "missing-node"),
		"--out", filepath.Join(root, "backups"),
		"--mdbx-map-size-gb", "1",
	}
	if os.Getenv("INNBC_DATABASE_BACKUP_TEST_MODE") == "json" {
		args = append(args, "--json")
	}

	/* START ALPHA CHANGE 3283 -- preserve a normal return as process success for the parent assertion -- */
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	os.Exit(0)
	/* FINISH ALPHA CHANGE 3283 -- preserve a normal return as process success for the parent assertion -- */
}

/* FINISH ALPHA CHANGE 2267 -- test backup CLI failure status in plain and JSON modes -- */
