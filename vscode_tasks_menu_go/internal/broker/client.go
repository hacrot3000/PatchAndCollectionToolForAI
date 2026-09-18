package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"bletonfc/vscode_tasks_menu/internal/session"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

type Client struct {
	workspace string
	info      Info
	http      *http.Client
	transport *http.Transport
}

type cancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelReadCloser) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

var _ session.Service = (*Client)(nil)

func NewClient(workspace string) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	info, err := Probe(ctx, workspace)
	if err != nil {
		return nil, err
	}
	transport := unixTransport(info.SocketPath)
	return &Client{
		workspace: workspace,
		info:      info,
		transport: transport,
		http:      &http.Client{Transport: transport},
	}, nil
}

func unixTransport(socketPath string) *http.Transport {
	dialer := &net.Dialer{Timeout: time.Second}
	return &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
}

func (c *Client) Close() {
	if c.transport != nil {
		c.transport.CloseIdleConnections()
	}
}

func (c *Client) endpoint(path string) string {
	return "http://session-broker" + path
}

func (c *Client) do(method, path string, body io.Reader, contentType string) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint(path), body)
	if err != nil {
		cancel()
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("session broker %s %s: %w", method, path, err)
	}
	resp.Body = &cancelReadCloser{ReadCloser: resp.Body, cancel: cancel}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	message := strings.TrimSpace(string(data))
	if message == "" {
		message = resp.Status
	}
	return nil, fmt.Errorf("session broker %s %s: %s", method, path, message)
}

func (c *Client) doJSON(method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	resp, err := c.do(method, path, body, "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if output == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(output)
}

func (c *Client) ListWithError() ([]session.Metadata, error) {
	var payload struct {
		Sessions []session.Metadata `json:"sessions"`
	}
	if err := c.doJSON(http.MethodGet, "/v1/sessions", nil, &payload); err != nil {
		return nil, err
	}
	return payload.Sessions, nil
}

func (c *Client) List() []session.Metadata {
	items, _ := c.ListWithError()
	return items
}

func (c *Client) Start(spec tasks.Execution) (session.Metadata, error) {
	var meta session.Metadata
	err := c.doJSON(http.MethodPost, "/v1/sessions", executionToWire(spec), &meta)
	return meta, err
}

func (c *Client) Metadata(id string) (session.Metadata, bool) {
	var meta session.Metadata
	if err := c.doJSON(http.MethodGet, "/v1/sessions/"+url.PathEscape(id), nil, &meta); err != nil {
		return session.Metadata{}, false
	}
	return meta, true
}

func (c *Client) Input(id string, data []byte) error {
	resp, err := c.do(http.MethodPost, "/v1/sessions/"+url.PathEscape(id)+"/input", bytes.NewReader(data), "application/octet-stream")
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func (c *Client) Resize(id string, rows, cols uint16) error {
	return c.doJSON(http.MethodPost, "/v1/sessions/"+url.PathEscape(id)+"/resize", ResizeRequest{Rows: rows, Cols: cols}, nil)
}

func (c *Client) action(id, action string) error {
	var meta session.Metadata
	return c.doJSON(http.MethodPost, "/v1/sessions/"+url.PathEscape(id)+"/"+action, nil, &meta)
}

func (c *Client) Stop(id string) error  { return c.action(id, "stop") }
func (c *Client) Kill(id string) error  { return c.action(id, "kill") }
func (c *Client) Clear(id string) error { return c.action(id, "clear") }

func (c *Client) Remove(id string) error {
	resp, err := c.do(http.MethodDelete, "/v1/sessions/"+url.PathEscape(id), nil, "")
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func (c *Client) CurrentCwd(id string) (string, error) {
	var payload struct {
		Cwd string `json:"cwd"`
	}
	if err := c.doJSON(http.MethodGet, "/v1/sessions/"+url.PathEscape(id)+"/cwd", nil, &payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.Cwd) == "" {
		return "", fmt.Errorf("terminal cwd unavailable")
	}
	return payload.Cwd, nil
}

func (c *Client) Subscribe(id string) ([]byte, <-chan []byte, func(), error) {
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("/v1/sessions/"+url.PathEscape(id)+"/stream"), nil)
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		cancel()
		return nil, nil, nil, fmt.Errorf("subscribe session broker: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		_ = resp.Body.Close()
		cancel()
		return nil, nil, nil, fmt.Errorf("subscribe session broker: %s", strings.TrimSpace(string(data)))
	}
	backlog, err := readFrame(resp.Body)
	if err != nil {
		_ = resp.Body.Close()
		cancel()
		return nil, nil, nil, fmt.Errorf("read broker backlog: %w", err)
	}
	stream := make(chan []byte, 256)
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			_ = resp.Body.Close()
		})
	}
	go func() {
		defer close(stream)
		defer stop()
		for {
			data, err := readFrame(resp.Body)
			if err != nil {
				return
			}
			stream <- data
		}
	}()
	return backlog, stream, stop, nil
}

func (c *Client) ShutdownBroker() error {
	return c.doJSON(http.MethodPost, "/v1/shutdown", nil, nil)
}
