/* START ALPHA CHANGE 2088 -- verify recovery CLI failures return nonzero status -- */
package recoverycheckpoint

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/command/helper"
	"github.com/stretchr/testify/require"
)

const recoveryFailureHelperEnvironment = "INNBC_RECOVERY_FAILURE_HELPER"

func TestRecoveryCheckpointFailureExitStatus(t *testing.T) {
	tests := []struct {
		name       string
		outputMode string
	}{
		{
			name:       "plain output",
			outputMode: "plain",
		},
		{
			name:       "JSON output",
			outputMode: "json",
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			process := exec.Command(
				os.Args[0],
				"-test.run=^TestRecoveryCheckpointFailureExitStatusHelper$",
			)
			process.Env = append(
				os.Environ(),
				recoveryFailureHelperEnvironment+"=1",
				"INNBC_RECOVERY_FAILURE_MODE="+test.outputMode,
				"INNBC_RECOVERY_FAILURE_ROOT="+root,
			)

			var standardOutput bytes.Buffer
			var standardError bytes.Buffer
			process.Stdout = &standardOutput
			process.Stderr = &standardError

			err := process.Run()

			var exitError *exec.ExitError
			require.True(t, errors.As(err, &exitError))
			require.Equal(t, 1, exitError.ExitCode())
			require.Empty(t, strings.TrimSpace(standardOutput.String()))

			errorOutput := strings.TrimSpace(standardError.String())
			require.NotEmpty(t, errorOutput)

			if test.outputMode == "json" {
				var decoded struct {
					Error string `json:"error"`
				}
				require.NoError(
					t,
					json.Unmarshal([]byte(errorOutput), &decoded),
				)
				require.NotEmpty(t, decoded.Error)
			}
		})
	}
}

func TestRecoveryCheckpointFailureExitStatusHelper(t *testing.T) {
	if os.Getenv(recoveryFailureHelperEnvironment) != "1" {
		return
	}

	root := os.Getenv("INNBC_RECOVERY_FAILURE_ROOT")
	cmd := GetCommand()
	helper.RegisterJSONOutputFlag(cmd)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	args := []string{
		"--chain",
		filepath.Join(root, "missing-genesis.json"),
		"--source-data-dir",
		filepath.Join(root, "missing-source"),
		"--snapshot-output-root",
		filepath.Join(root, "snapshots"),
		"--archive-output-root",
		filepath.Join(root, "missing-archive"),
		"--minimum-retention-blocks",
		"2",
		"--mdbx-map-size-gb",
		"1",
	}
	if os.Getenv("INNBC_RECOVERY_FAILURE_MODE") == "json" {
		args = append(args, "--json")
	}

	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		os.Exit(2)
	}

	os.Exit(0)
}

/* FINISH ALPHA CHANGE 2088 -- verify recovery CLI failures return nonzero status -- */
