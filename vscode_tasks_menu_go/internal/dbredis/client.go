package dbredis

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"
)

type Client struct {
	conn    net.Conn
	reader  *bufio.Reader
	timeout time.Duration
	mu      sync.Mutex
}

func Dial(ctx context.Context, config Config) (*Client, error) {
	if ctx == nil {
		return nil, errors.New("Redis dial context is required")
	}
	address := net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	dialer := net.Dialer{Timeout: config.ConnectTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("connect Redis %s: %w", address, err)
	}
	return &Client{
		conn:    conn,
		reader:  bufio.NewReaderSize(conn, 64<<10),
		timeout: config.CommandTimeout,
	}, nil
}

func (c *Client) Do(ctx context.Context, args ...string) (Value, error) {
	if c == nil || c.conn == nil || c.reader == nil {
		return Value{}, errors.New("Redis client is not connected")
	}
	if ctx == nil {
		return Value{}, errors.New("Redis command context is required")
	}
	if len(args) == 0 {
		return Value{}, errors.New("Redis command is required")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	deadline := time.Now().Add(c.timeout)
	if deadlineFromContext, ok := ctx.Deadline(); ok && deadlineFromContext.Before(deadline) {
		deadline = deadlineFromContext
	}
	if err := c.conn.SetDeadline(deadline); err != nil {
		return Value{}, fmt.Errorf("set Redis command deadline: %w", err)
	}
	stopCancelWatch := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = c.conn.SetDeadline(time.Now())
		case <-stopCancelWatch:
		}
	}()
	defer func() {
		close(stopCancelWatch)
		_ = c.conn.SetDeadline(time.Time{})
	}()

	if err := WriteCommand(c.conn, args); err != nil {
		if ctx.Err() != nil {
			return Value{}, ctx.Err()
		}
		return Value{}, err
	}
	value, err := ReadValue(c.reader)
	if err != nil {
		if ctx.Err() != nil {
			return Value{}, ctx.Err()
		}
		return Value{}, err
	}
	if value.Kind == KindError {
		return Value{}, &RedisError{Message: value.Text}
	}
	return value, nil
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	err := c.conn.Close()
	c.conn = nil
	c.reader = nil
	return err
}
