package ftpclient

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	maxControlLineBytes  = 8 << 10
	maxControlReplyBytes = 64 << 10
	maxListingBytes      = 4 << 20
	MaxListEntries       = 5000
)

var ErrResumeUnsupported = errors.New("ftp server does not support transfer resume")

type Reply struct {
	Code int
	Text string
}

type Client struct {
	conn    net.Conn
	reader  *bufio.Reader
	writer  *bufio.Writer
	peerIP  net.IP
	timeout time.Duration
}

func Dial(ctx context.Context, address, username, password string, timeout time.Duration) (*Client, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if err := validateCommandArg("ftp address", address); err != nil {
		return nil, err
	}
	if err := validateCommandArg("ftp username", username); err != nil {
		return nil, err
	}
	if err := validateCommandArgAllowEmpty("ftp password", password); err != nil {
		return nil, err
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("ftp connect: %w", err)
	}
	client := &Client{
		conn: conn, reader: bufio.NewReaderSize(conn, 32<<10),
		writer: bufio.NewWriterSize(conn, 8<<10), timeout: timeout,
	}
	if tcp, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		client.peerIP = append(net.IP(nil), tcp.IP...)
	}
	if err := client.applyDeadline(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	greeting, err := client.readReply()
	if err != nil {
		conn.Close()
		return nil, err
	}
	if greeting.Code != 220 {
		conn.Close()
		return nil, fmt.Errorf("ftp greeting rejected: %d %s", greeting.Code, greeting.Text)
	}
	userReply, err := client.command(ctx, "USER "+username)
	if err != nil {
		conn.Close()
		return nil, err
	}
	switch userReply.Code {
	case 230:
		// Already logged in.
	case 331:
		passReply, err := client.command(ctx, "PASS "+password)
		if err != nil {
			conn.Close()
			return nil, err
		}
		if passReply.Code != 230 {
			conn.Close()
			return nil, fmt.Errorf("ftp login rejected: %d %s", passReply.Code, passReply.Text)
		}
	default:
		conn.Close()
		return nil, fmt.Errorf("ftp USER rejected: %d %s", userReply.Code, userReply.Text)
	}
	typeReply, err := client.command(ctx, "TYPE I")
	if err != nil {
		conn.Close()
		return nil, err
	}
	if typeReply.Code != 200 {
		conn.Close()
		return nil, fmt.Errorf("ftp TYPE I rejected: %d %s", typeReply.Code, typeReply.Text)
	}
	return client, nil
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	_ = c.applyDeadline(context.Background())
	_, _ = c.command(context.Background(), "QUIT")
	return c.conn.Close()
}

func (c *Client) Noop(ctx context.Context) error {
	reply, err := c.command(ctx, "NOOP")
	if err != nil {
		return err
	}
	if reply.Code/100 != 2 {
		return fmt.Errorf("ftp NOOP rejected: %d %s", reply.Code, reply.Text)
	}
	return nil
}

func (c *Client) Pwd(ctx context.Context) (string, error) {
	reply, err := c.command(ctx, "PWD")
	if err != nil {
		return "", err
	}
	if reply.Code != 257 {
		return "", fmt.Errorf("ftp PWD rejected: %d %s", reply.Code, reply.Text)
	}
	text := strings.TrimSpace(reply.Text)
	if !strings.HasPrefix(text, "\"") {
		return text, nil
	}
	var b strings.Builder
	for i := 1; i < len(text); i++ {
		if text[i] != '"' {
			b.WriteByte(text[i])
			continue
		}
		if i+1 < len(text) && text[i+1] == '"' {
			b.WriteByte('"')
			i++
			continue
		}
		return b.String(), nil
	}
	return "", errors.New("ftp PWD returned an unterminated quoted path")
}

func (c *Client) Mkdir(ctx context.Context, path string) error {
	return c.expect2xx(ctx, "MKD "+mustArg(path))
}

func (c *Client) Delete(ctx context.Context, path string, directory bool) error {
	command := "DELE "
	if directory {
		command = "RMD "
	}
	return c.expect2xx(ctx, command+mustArg(path))
}

func (c *Client) Rename(ctx context.Context, oldPath, newPath string) error {
	if err := validateCommandArg("ftp old path", oldPath); err != nil {
		return err
	}
	if err := validateCommandArg("ftp new path", newPath); err != nil {
		return err
	}
	reply, err := c.command(ctx, "RNFR "+oldPath)
	if err != nil {
		return err
	}
	if reply.Code != 350 {
		return fmt.Errorf("ftp RNFR rejected: %d %s", reply.Code, reply.Text)
	}
	return c.expect2xx(ctx, "RNTO "+newPath)
}

func (c *Client) List(ctx context.Context, path string) ([]Entry, error) {
	if err := validateCommandArg("ftp remote path", path); err != nil {
		return nil, err
	}
	data, final, err := c.dataBytes(ctx, commandWithOptionalArg("MLSD", path), maxListingBytes)
	if err == nil {
		entries, parseErr := ParseMLSD(string(data))
		if parseErr != nil {
			return nil, parseErr
		}
		return entries, nil
	}
	if final.Code != 500 && final.Code != 501 && final.Code != 502 && final.Code != 504 {
		return nil, err
	}
	data, _, err = c.dataBytes(ctx, commandWithOptionalArg("LIST", path), maxListingBytes)
	if err != nil {
		return nil, err
	}
	return ParseLIST(string(data))
}

func (c *Client) setRestartOffset(ctx context.Context, offset int64) error {
	if offset <= 0 {
		return nil
	}
	reply, err := c.command(ctx, "REST "+strconv.FormatInt(offset, 10))
	if err != nil {
		return err
	}
	if reply.Code == 350 {
		return nil
	}
	if reply.Code == 500 || reply.Code == 501 || reply.Code == 502 || reply.Code == 504 {
		return fmt.Errorf("%w: %d %s", ErrResumeUnsupported, reply.Code, reply.Text)
	}
	return fmt.Errorf("ftp REST rejected: %d %s", reply.Code, reply.Text)
}

func (c *Client) Retrieve(ctx context.Context, remotePath string, dst io.Writer) error {
	return c.RetrieveFrom(ctx, remotePath, 0, dst)
}

func (c *Client) RetrieveFrom(ctx context.Context, remotePath string, offset int64, dst io.Writer) error {
	if err := validateCommandArg("ftp remote path", remotePath); err != nil {
		return err
	}
	if offset < 0 {
		return errors.New("ftp resume offset must be non-negative")
	}
	if err := c.setRestartOffset(ctx, offset); err != nil {
		return err
	}
	return c.dataStream(ctx, "RETR "+remotePath, func(conn net.Conn) error {
		_, err := io.Copy(dst, conn)
		return err
	})
}

func (c *Client) Store(ctx context.Context, remotePath string, src io.Reader) error {
	return c.StoreFrom(ctx, remotePath, 0, src)
}

func (c *Client) StoreFrom(ctx context.Context, remotePath string, offset int64, src io.Reader) error {
	if err := validateCommandArg("ftp remote path", remotePath); err != nil {
		return err
	}
	if offset < 0 {
		return errors.New("ftp resume offset must be non-negative")
	}
	if err := c.setRestartOffset(ctx, offset); err != nil {
		return err
	}
	return c.dataStream(ctx, "STOR "+remotePath, func(conn net.Conn) error {
		_, err := io.Copy(conn, src)
		if tcp, ok := conn.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		return err
	})
}

func (c *Client) expect2xx(ctx context.Context, command string) error {
	reply, err := c.command(ctx, command)
	if err != nil {
		return err
	}
	if reply.Code/100 != 2 {
		return fmt.Errorf("ftp command rejected: %d %s", reply.Code, reply.Text)
	}
	return nil
}

func (c *Client) dataBytes(ctx context.Context, command string, limit int64) ([]byte, Reply, error) {
	var builder strings.Builder
	err := c.dataStream(ctx, command, func(conn net.Conn) error {
		limited := io.LimitReader(conn, limit+1)
		data, err := io.ReadAll(limited)
		if err != nil {
			return err
		}
		if int64(len(data)) > limit {
			return fmt.Errorf("ftp data exceeds %d bytes", limit)
		}
		builder.Write(data)
		return nil
	})
	if err != nil {
		var commandErr *dataCommandError
		if errors.As(err, &commandErr) {
			return nil, commandErr.reply, err
		}
		return nil, Reply{}, err
	}
	return []byte(builder.String()), Reply{Code: 226}, nil
}

type dataCommandError struct {
	reply Reply
}

func (e *dataCommandError) Error() string {
	return fmt.Sprintf("ftp data command rejected: %d %s", e.reply.Code, e.reply.Text)
}

func (c *Client) dataStream(ctx context.Context, command string, transfer func(net.Conn) error) error {
	if err := validateCommandLine(command); err != nil {
		return err
	}
	dataConn, err := c.openPassive(ctx)
	if err != nil {
		return err
	}
	if err := c.applyDeadline(ctx); err != nil {
		dataConn.Close()
		return err
	}
	if _, err := c.writeCommand(command); err != nil {
		dataConn.Close()
		return err
	}
	preliminary, err := c.readReply()
	if err != nil {
		dataConn.Close()
		return err
	}
	if preliminary.Code != 125 && preliminary.Code != 150 {
		dataConn.Close()
		return &dataCommandError{reply: preliminary}
	}
	if err := applyConnDeadline(dataConn, ctx, c.timeout); err != nil {
		dataConn.Close()
		return err
	}
	transferErr := transfer(dataConn)
	closeErr := dataConn.Close()
	final, replyErr := c.readReply()
	if transferErr != nil {
		return transferErr
	}
	if closeErr != nil {
		return closeErr
	}
	if replyErr != nil {
		return replyErr
	}
	if final.Code != 226 && final.Code != 250 {
		return &dataCommandError{reply: final}
	}
	return nil
}

func (c *Client) openPassive(ctx context.Context) (net.Conn, error) {
	reply, err := c.command(ctx, "EPSV")
	if err != nil {
		return nil, err
	}
	if reply.Code == 229 {
		port, err := parseEPSVPort(reply.Text)
		if err != nil {
			return nil, err
		}
		return c.dialData(ctx, port)
	}
	if reply.Code != 500 && reply.Code != 501 && reply.Code != 502 && reply.Code != 522 {
		return nil, fmt.Errorf("ftp EPSV rejected: %d %s", reply.Code, reply.Text)
	}
	if c.peerIP == nil || c.peerIP.To4() == nil {
		return nil, errors.New("ftp server does not support EPSV and PASV fallback requires IPv4")
	}
	pasv, err := c.command(ctx, "PASV")
	if err != nil {
		return nil, err
	}
	if pasv.Code != 227 {
		return nil, fmt.Errorf("ftp PASV rejected: %d %s", pasv.Code, pasv.Text)
	}
	port, err := parsePASVPort(pasv.Text)
	if err != nil {
		return nil, err
	}
	// Deliberately use the control peer address instead of blindly trusting
	// the server-provided PASV address.
	return c.dialData(ctx, port)
}

func (c *Client) dialData(ctx context.Context, port int) (net.Conn, error) {
	if c.peerIP == nil {
		return nil, errors.New("ftp control peer address is unavailable")
	}
	dialer := net.Dialer{Timeout: c.timeout}
	address := net.JoinHostPort(c.peerIP.String(), strconv.Itoa(port))
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("ftp data connect: %w", err)
	}
	return conn, nil
}

func (c *Client) command(ctx context.Context, command string) (Reply, error) {
	if err := validateCommandLine(command); err != nil {
		return Reply{}, err
	}
	if err := c.applyDeadline(ctx); err != nil {
		return Reply{}, err
	}
	if _, err := c.writeCommand(command); err != nil {
		return Reply{}, err
	}
	return c.readReply()
}

func (c *Client) writeCommand(command string) (int, error) {
	n, err := c.writer.WriteString(command + "\r\n")
	if err != nil {
		return n, fmt.Errorf("ftp write command: %w", err)
	}
	if err := c.writer.Flush(); err != nil {
		return n, fmt.Errorf("ftp flush command: %w", err)
	}
	return n, nil
}

func (c *Client) readReply() (Reply, error) {
	first, err := readBoundedLine(c.reader, maxControlLineBytes)
	if err != nil {
		return Reply{}, fmt.Errorf("ftp read reply: %w", err)
	}
	if len(first) < 3 {
		return Reply{}, fmt.Errorf("malformed ftp reply %q", first)
	}
	code, err := strconv.Atoi(first[:3])
	if err != nil || code < 100 || code > 599 {
		return Reply{}, fmt.Errorf("malformed ftp reply code %q", first)
	}
	total := len(first)
	lines := []string{replyPayload(first)}
	if len(first) >= 4 && first[3] == '-' {
		prefix := first[:3] + " "
		for {
			line, err := readBoundedLine(c.reader, maxControlLineBytes)
			if err != nil {
				return Reply{}, fmt.Errorf("ftp read multiline reply: %w", err)
			}
			total += len(line)
			if total > maxControlReplyBytes {
				return Reply{}, fmt.Errorf("ftp control reply exceeds %d bytes", maxControlReplyBytes)
			}
			lines = append(lines, replyPayload(line))
			if strings.HasPrefix(line, prefix) {
				break
			}
		}
	}
	return Reply{Code: code, Text: strings.TrimSpace(strings.Join(lines, "\n"))}, nil
}

func readBoundedLine(r *bufio.Reader, limit int) (string, error) {
	var b []byte
	for {
		part, err := r.ReadSlice('\n')
		if len(b)+len(part) > limit {
			return "", fmt.Errorf("line exceeds %d bytes", limit)
		}
		b = append(b, part...)
		switch {
		case err == nil:
			return strings.TrimRight(string(b), "\r\n"), nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && len(b) > 0:
			return strings.TrimRight(string(b), "\r\n"), nil
		default:
			return "", err
		}
	}
}

func replyPayload(line string) string {
	if len(line) <= 4 {
		return ""
	}
	return line[4:]
}

func (c *Client) applyDeadline(ctx context.Context) error {
	return applyConnDeadline(c.conn, ctx, c.timeout)
}

func applyConnDeadline(conn net.Conn, ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	return conn.SetDeadline(deadline)
}

func parseEPSVPort(text string) (int, error) {
	start := strings.LastIndex(text, "(")
	end := strings.LastIndex(text, ")")
	if start < 0 || end <= start+4 {
		return 0, fmt.Errorf("malformed EPSV reply %q", text)
	}
	body := text[start+1 : end]
	if len(body) < 5 {
		return 0, fmt.Errorf("malformed EPSV reply %q", text)
	}
	delim := body[0]
	parts := strings.Split(body, string(delim))
	if len(parts) < 5 {
		return 0, fmt.Errorf("malformed EPSV reply %q", text)
	}
	port, err := strconv.Atoi(parts[len(parts)-2])
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("malformed EPSV port in %q", text)
	}
	return port, nil
}

func parsePASVPort(text string) (int, error) {
	start := strings.LastIndex(text, "(")
	end := strings.LastIndex(text, ")")
	if start < 0 || end <= start+1 {
		return 0, fmt.Errorf("malformed PASV reply %q", text)
	}
	parts := strings.Split(text[start+1:end], ",")
	if len(parts) != 6 {
		return 0, fmt.Errorf("malformed PASV reply %q", text)
	}
	values := make([]int, 6)
	for i, part := range parts {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || value < 0 || value > 255 {
			return 0, fmt.Errorf("malformed PASV reply %q", text)
		}
		values[i] = value
	}
	port := values[4]*256 + values[5]
	if port < 1 {
		return 0, fmt.Errorf("malformed PASV port in %q", text)
	}
	return port, nil
}

func commandWithOptionalArg(command, arg string) string {
	if strings.TrimSpace(arg) == "" || arg == "." {
		return command
	}
	return command + " " + arg
}

func validateCommandLine(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("ftp command is empty")
	}
	if strings.ContainsAny(value, "\x00\r\n") {
		return errors.New("ftp command contains control characters")
	}
	return nil
}

func validateCommandArg(label, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", label)
	}
	return validateCommandArgAllowEmpty(label, value)
}

func validateCommandArgAllowEmpty(label, value string) error {
	if strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("%s contains control characters", label)
	}
	return nil
}

func mustArg(value string) string {
	return value
}
