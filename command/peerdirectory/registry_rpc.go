/* START ALPHA CHANGE 2228 -- bound peer-directory registry HTTP calls -- */
package peerdirectory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc/codec"
)

const (
	registryRPCTimeout    = 5 * time.Second
	registryResponseLimit = 4 * 1024 * 1024
)

type registryRPCClient struct {
	url    string
	client *http.Client
}

func newRegistryRPCClient(endpoint string) (*registryRPCClient, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("peer-directory registry endpoint must use HTTP or HTTPS")
	}

	return &registryRPCClient{
		url: endpoint,
		client: &http.Client{
			Timeout: registryRPCTimeout,
		},
	}, nil
}

func (c *registryRPCClient) call(method string, result interface{}, params ...interface{}) error {
	encodedParams, err := json.Marshal(params)
	if err != nil {
		return err
	}

	body, err := json.Marshal(codec.Request{
		JsonRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  encodedParams,
	})
	if err != nil {
		return err
	}

	request, err := http.NewRequest(http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(response.Body, registryResponseLimit+1))
	if err != nil {
		return err
	}
	if len(payload) > registryResponseLimit {
		return fmt.Errorf("registry RPC response exceeds %d bytes", registryResponseLimit)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("registry RPC returned HTTP status %d", response.StatusCode)
	}

	var decoded codec.Response
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return err
	}
	if decoded.ID != 1 {
		return fmt.Errorf("registry RPC response ID does not match request")
	}
	if decoded.Error != nil {
		return decoded.Error
	}

	return json.Unmarshal(decoded.Result, result)
}

func (c *registryRPCClient) Call(msg *ethgo.CallMsg, block ethgo.BlockNumber) (string, error) {
	var result string
	if err := c.call("eth_call", &result, msg, block.String()); err != nil {
		return "", err
	}
	if !strings.HasPrefix(result, "0x") {
		return "", fmt.Errorf("registry eth_call result has no hexadecimal prefix")
	}

	return result, nil
}

func (c *registryRPCClient) BlockNumber() (uint64, error) {
	var result string
	if err := c.call("eth_blockNumber", &result); err != nil {
		return 0, err
	}
	if !strings.HasPrefix(result, "0x") {
		return 0, fmt.Errorf("registry block number has no hexadecimal prefix")
	}

	return strconv.ParseUint(result[2:], 16, 64)
}

/* FINISH ALPHA CHANGE 2228 -- bound peer-directory registry HTTP calls -- */
