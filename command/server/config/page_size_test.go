/* START ALPHA CHANGE 3488 -- verify page settings in supported configuration formats -- */
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMDBXPageSize_ConfigFiles(t *testing.T) {
	for _, format := range []struct {
		extension string
		template  string
	}{
		{"json", `{"history":{"mdbx_page_size_bytes":%s}}`},
		{"yaml", "history:\n  mdbx_page_size_bytes: %s\n"},
		{"hcl", "history {\n  mdbx_page_size_bytes = %s\n}\n"},
	} {
		for _, value := range []string{"4096", "8192", "0", "4097"} {
			t.Run(format.extension+"/"+value, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "server."+format.extension)
				/* START ALPHA CHANGE 3494 -- format configuration fixtures directly into bytes -- */
				require.NoError(t, os.WriteFile(
					path, fmt.Appendf(nil, format.template, value), 0600,
				))
				/* FINISH ALPHA CHANGE 3494 -- format configuration fixtures directly into bytes -- */
				loaded, err := ReadConfigFile(path)
				if value == "0" || value == "4097" {
					require.Error(t, err)
					return
				}
				require.NoError(t, err)
				require.Equal(t, value, loaded.History.MDBXPageSizeBytes.String())
			})
		}
	}

	path := filepath.Join(t.TempDir(), "server.json")
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0600))
	loaded, err := ReadConfigFile(path)
	require.NoError(t, err)
	require.Equal(t, "4096", loaded.History.MDBXPageSizeBytes.String())

	/* START ALPHA CHANGE 3498 -- verify HCL defaults aliases and invalid page settings -- */
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{"omitted", "", "4096"},
		{"empty_history", "History {}", "4096"},
		{"blank_value", `History { mdbx_page_size_bytes = "" }`, "4096"},
		{"field_name", "History { MDBXPageSizeBytes = 8192 }", "8192"},
		{"quoted_value", `History { mdbx_page_size_bytes = "8192" }`, "8192"},
		{"field_zero", "History { MDBXPageSizeBytes = 0 }", ""},
		{"invalid_text", `History { mdbx_page_size_bytes = "bad" }`, ""},
		{"invalid_type", "History { mdbx_page_size_bytes = true }", ""},
		{
			"duplicate_spellings",
			"History {\n mdbx_page_size_bytes = 4096\n MDBXPageSizeBytes = 8192\n}",
			"",
		},
	} {
		t.Run("hcl_policy/"+tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "server.hcl")
			require.NoError(t, os.WriteFile(path, []byte(tc.body), 0600))

			loaded, err := ReadConfigFile(path)
			if tc.want == "" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, loaded.History.MDBXPageSizeBytes.String())
		})
	}
	/* FINISH ALPHA CHANGE 3498 -- verify HCL defaults aliases and invalid page settings -- */
}

/* FINISH ALPHA CHANGE 3488 -- verify page settings in supported configuration formats -- */
