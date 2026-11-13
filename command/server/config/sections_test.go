/* START ALPHA CHANGE 3307 -- test required configuration sections and preserve loader behavior -- */
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func readConfigSectionsFixture(
	t *testing.T,
	suffix string,
	body string,
) (*Config, error) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "server"+suffix)
	require.NoError(t, os.WriteFile(path, []byte(body), 0600))

	var (
		loaded *Config
		err    error
	)
	require.NotPanics(t, func() {
		loaded, err = ReadConfigFile(path)
	})

	return loaded, err
}

func TestConfigSections_ExplicitNull(t *testing.T) {
	for _, suffix := range []string{".json", ".yaml", ".yml"} {
		values := []string{"null"}
		if suffix != ".json" {
			values = append(values, "~", "")
		}

		for _, section := range []string{"network", "telemetry", "tx_pool"} {
			for _, value := range values {
				t.Run(fmt.Sprintf(
					"%s/%s/value=%q", suffix, section, value,
				), func(t *testing.T) {
					body := fmt.Sprintf("{%q: %s}", section, value)
					if suffix != ".json" {
						body = fmt.Sprintf("%s: %s\n", section, value)
					}

					loaded, err := readConfigSectionsFixture(t, suffix, body)

					require.Nil(t, loaded)
					require.EqualError(t, err, fmt.Sprintf(
						"invalid server configuration: section %q must not be null",
						section,
					))
				})
			}
		}
	}
}

func TestConfigSections_DefaultsAndPartialObjects(t *testing.T) {
	for _, suffix := range []string{".json", ".yaml", ".yml"} {
		for _, scenario := range []string{"omitted", "empty", "partial"} {
			t.Run(suffix+"/"+scenario, func(t *testing.T) {
				want := DefaultConfig()

				// ReadConfigFile deliberately uses a separate network object
				// with unset peer limits rather than DefaultConfig().Network.
				want.Network = &Network{
					MaxPeers:         -1,
					MaxInboundPeers:  -1,
					MaxOutboundPeers: -1,
				}

				body := "{}"
				switch scenario {
				case "empty":
					if suffix == ".json" {
						body = `{"network":{},"telemetry":{},"tx_pool":{}}`
					} else {
						body = "network: {}\ntelemetry: {}\ntx_pool: {}\n"
					}

				case "partial":
					if suffix == ".json" {
						body = `{
							"network":{"max_outbound_peers":3},
							"telemetry":{"prometheus_addr":"127.0.0.1:9090"},
							"tx_pool":{"max_slots":17}
						}`
					} else {
						body = "network:\n  max_outbound_peers: 3\n" +
							"telemetry:\n  prometheus_addr: 127.0.0.1:9090\n" +
							"tx_pool:\n  max_slots: 17\n"
					}

					want.Network.MaxOutboundPeers = 3
					want.Telemetry.PrometheusAddr = "127.0.0.1:9090"
					want.TxPool.MaxSlots = 17
				}

				loaded, err := readConfigSectionsFixture(t, suffix, body)
				require.NoError(t, err)
				require.Equal(t, want, loaded)
				require.NoError(t, loaded.ValidateSections())
			})
		}
	}
}

func TestConfigSections_DecoderErrorsPreserved(t *testing.T) {
	for _, suffix := range []string{".json", ".yaml", ".yml"} {
		for _, section := range []string{"network", "telemetry", "tx_pool"} {
			t.Run(suffix+"/"+section, func(t *testing.T) {
				body := fmt.Sprintf("{%q: true}", section)
				if suffix != ".json" {
					body = fmt.Sprintf("%s: true\n", section)
				}

				loaded, err := readConfigSectionsFixture(t, suffix, body)
				require.Nil(t, loaded)
				require.Error(t, err)

				if suffix == ".json" {
					var decoderError *json.UnmarshalTypeError
					require.ErrorAs(t, err, &decoderError)
				} else {
					var decoderError *yaml.TypeError
					require.ErrorAs(t, err, &decoderError)
				}
			})
		}
	}
}

func TestConfigSections_HCLBehaviorPreserved(t *testing.T) {
	// Use the field-name syntax supported by the existing HCL decoder.
	body := `
Network {
	MaxPeers = 9
	MaxInboundPeers = -1
	MaxOutboundPeers = -1
}
Telemetry {
	PrometheusAddr = "127.0.0.1:9090"
}
TxPool {}
`

	loaded, err := readConfigSectionsFixture(t, ".hcl", body)
	require.NoError(t, err)

	want := DefaultConfig()
	want.Network = &Network{
		MaxPeers:         9,
		MaxInboundPeers:  -1,
		MaxOutboundPeers: -1,
	}
	want.Telemetry = &Telemetry{
		PrometheusAddr: "127.0.0.1:9090",
	}

	// Unlike JSON/YAML merging, the pinned HCL decoder replaces section
	// pointers. An explicit empty TxPool block therefore remains zero-valued.
	want.TxPool = &TxPool{}

	require.Equal(t, want, loaded)
	require.NoError(t, loaded.ValidateSections())
}

func TestConfigSections_OptionalNullPoliciesPreserved(t *testing.T) {
	for _, suffix := range []string{".json", ".yaml", ".yml"} {
		t.Run(suffix, func(t *testing.T) {
			body := `{"history":null,"json_rpc_admission":null}`
			if suffix != ".json" {
				body = "history: null\njson_rpc_admission: null\n"
			}

			loaded, err := readConfigSectionsFixture(t, suffix, body)
			require.NoError(t, err)
			require.NoError(t, loaded.ValidateSections())

			require.Nil(t, loaded.History)
			require.Nil(t, loaded.JSONRPCAdmission)
			require.NoError(t, loaded.JSONRPCAdmission.Validate())
		})
	}
}

/* FINISH ALPHA CHANGE 3307 -- test required configuration sections and preserve loader behavior -- */
