/* START ALPHA CHANGE 2087 -- verify archive CLI failures return nonzero status -- */
package archive

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

const archiveFailureHelperEnvironment = "INNBC_ARCHIVE_FAILURE_HELPER"

func TestArchiveFailureExitStatus(t *testing.T) {
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
				"-test.run=^TestArchiveFailureExitStatusHelper$",
			)
			process.Env = append(
				os.Environ(),
				archiveFailureHelperEnvironment+"=1",
				"INNBC_ARCHIVE_FAILURE_MODE="+test.outputMode,
				"INNBC_ARCHIVE_FAILURE_ROOT="+root,
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

func TestArchiveFailureExitStatusHelper(t *testing.T) {
	if os.Getenv(archiveFailureHelperEnvironment) != "1" {
		return
	}

	root := os.Getenv("INNBC_ARCHIVE_FAILURE_ROOT")
	cmd := GetCommand()
	helper.RegisterJSONOutputFlag(cmd)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	args := []string{
		"--chain",
		filepath.Join(root, "missing-genesis.json"),
		"--source-data-dir",
		filepath.Join(root, "missing-source"),
		"--output-root",
		filepath.Join(root, "archive"),
		"--segment-blocks",
		"2",
		"--max-index-records",
		"10",
		"--mdbx-map-size-gb",
		"1",
	}
	if os.Getenv("INNBC_ARCHIVE_FAILURE_MODE") == "json" {
		args = append(args, "--json")
	}

	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		os.Exit(2)
	}

	os.Exit(0)
}

/* FINISH ALPHA CHANGE 2087 -- verify archive CLI failures return nonzero status -- */
