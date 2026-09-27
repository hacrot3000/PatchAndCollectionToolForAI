package dbadapter

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

const maxDiagnosticBytes = 64 << 10

type responseResult struct {
	envelope Envelope
	err      error
}

type Process struct {
	manifest Manifest
	cmd      *exec.Cmd
	stdin    io.WriteCloser

	writeMu sync.Mutex

	mu       sync.Mutex
	pending  map[string]chan responseResult
	waitErr  error
	stderr   boundedDiagnostic
	done     chan struct{}
	doneOnce sync.Once
}

type ProcessOptions struct {
	Dir string
	Env []string
}

func StartProcess(ctx context.Context, manifest Manifest, options ProcessOptions) (*Process, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	if ctx == nil {
		return nil, errors.New("database adapter context is required")
	}

	cmd := exec.CommandContext(ctx, manifest.Command, manifest.Args...)
	if strings.TrimSpace(options.Dir) != "" {
		cmd.Dir = options.Dir
	}
	if options.Env != nil {
		cmd.Env = append([]string(nil), options.Env...)
	} else {
		cmd.Env = os.Environ()
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open database adapter stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("open database adapter stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("open database adapter stderr: %w", err)
	}

	p := &Process{
		manifest: manifest,
		cmd:      cmd,
		stdin:    stdin,
		pending:  make(map[string]chan responseResult),
		done:     make(chan struct{}),
		stderr:   boundedDiagnostic{remaining: maxDiagnosticBytes},
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, fmt.Errorf("start database adapter %q: %w", manifest.ID, err)
	}

	stdoutDone := make(chan struct{})
	stderrDone := make(chan struct{})
	go func() {
		defer close(stdoutDone)
		p.readStdout(stdout)
	}()
	go func() {
		defer close(stderrDone)
		p.readStderr(stderr)
	}()
	go p.waitAfterPipes(stdoutDone, stderrDone)
	return p, nil
}

func (p *Process) Manifest() Manifest {
	return p.manifest
}

func (p *Process) Request(ctx context.Context, operation Operation, payload interface{}, sessionID string) (Envelope, error) {
	if p == nil {
		return Envelope{}, errors.New("database adapter process is nil")
	}
	if ctx == nil {
		return Envelope{}, errors.New("database adapter request context is required")
	}
	if !p.manifest.Capabilities.Supports(operation) {
		return Envelope{}, fmt.Errorf("database adapter %q does not support operation %q", p.manifest.ID, operation)
	}

	requestID, err := newRequestID()
	if err != nil {
		return Envelope{}, err
	}
	request, err := NewRequest(requestID, operation, payload)
	if err != nil {
		return Envelope{}, err
	}
	request.SessionID = strings.TrimSpace(sessionID)
	if err := ValidateEnvelope(request); err != nil {
		return Envelope{}, err
	}

	responseCh := make(chan responseResult, 1)
	p.mu.Lock()
	select {
	case <-p.done:
		err := p.waitErr
		p.mu.Unlock()
		if err == nil {
			err = errors.New("database adapter process is not running")
		}
		return Envelope{}, err
	default:
	}
	p.pending[requestID] = responseCh
	p.mu.Unlock()

	if err := p.writeEnvelope(request); err != nil {
		p.removePending(requestID)
		p.fail(err)
		return Envelope{}, err
	}

	select {
	case result := <-responseCh:
		return result.envelope, result.err
	case <-ctx.Done():
		p.removePending(requestID)
		_ = p.Close()
		return Envelope{}, ctx.Err()
	case <-p.done:
		p.removePending(requestID)
		p.mu.Lock()
		err := p.waitErr
		p.mu.Unlock()
		if err == nil {
			err = errors.New("database adapter process exited")
		}
		return Envelope{}, err
	}
}

func (p *Process) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	cmd := p.cmd
	p.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	select {
	case <-p.done:
		return nil
	default:
	}
	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("kill database adapter %q: %w", p.manifest.ID, err)
	}
	return nil
}

func (p *Process) Done() <-chan struct{} {
	if p == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return p.done
}

func (p *Process) WaitError() error {
	if p == nil {
		return errors.New("database adapter process is nil")
	}
	<-p.done
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.waitErr
}

func (p *Process) Diagnostics() string {
	if p == nil {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stderr.String()
}

func (p *Process) writeEnvelope(env Envelope) error {
	if err := ValidateEnvelope(env); err != nil {
		return err
	}
	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("encode database adapter request: %w", err)
	}
	if len(data)+1 > MaxMessageBytes {
		return fmt.Errorf("database adapter request exceeds %d bytes", MaxMessageBytes)
	}
	data = append(data, '\n')

	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	if _, err := p.stdin.Write(data); err != nil {
		return fmt.Errorf("write database adapter request: %w", err)
	}
	return nil
}

func (p *Process) readStdout(stdout io.ReadCloser) {
	defer stdout.Close()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), MaxMessageBytes)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		var env Envelope
		if err := json.Unmarshal(line, &env); err != nil {
			p.fail(fmt.Errorf("database adapter emitted invalid JSON: %w", err))
			return
		}
		if err := ValidateEnvelope(env); err != nil {
			p.fail(fmt.Errorf("database adapter emitted invalid protocol message: %w", err))
			return
		}
		if env.Type != "response" || env.RequestID == "" {
			p.fail(fmt.Errorf("database adapter emitted unexpected %q message", env.Type))
			return
		}

		p.mu.Lock()
		ch, ok := p.pending[env.RequestID]
		if ok {
			delete(p.pending, env.RequestID)
		}
		p.mu.Unlock()
		if !ok {
			p.fail(fmt.Errorf("database adapter emitted response for unknown request %q", env.RequestID))
			return
		}
		if env.Error != nil {
			ch <- responseResult{err: fmt.Errorf("%s: %s", env.Error.Code, env.Error.Message)}
		} else {
			ch <- responseResult{envelope: env}
		}
	}
	if err := scanner.Err(); err != nil {
		p.fail(fmt.Errorf("read database adapter stdout: %w", err))
	}
}

func (p *Process) readStderr(stderr io.ReadCloser) {
	defer stderr.Close()
	buffer := make([]byte, 4096)
	for {
		n, err := stderr.Read(buffer)
		if n > 0 {
			p.mu.Lock()
			_, _ = p.stderr.Write(buffer[:n])
			p.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (p *Process) waitAfterPipes(stdoutDone, stderrDone <-chan struct{}) {
	<-stdoutDone
	<-stderrDone
	err := p.cmd.Wait()
	if err != nil {
		p.finish(fmt.Errorf("database adapter %q exited: %w", p.manifest.ID, err))
		return
	}
	p.finish(nil)
}

func (p *Process) fail(err error) {
	if p == nil || err == nil {
		return
	}
	_ = p.Close()
	p.finish(err)
}

func (p *Process) finish(err error) {
	p.doneOnce.Do(func() {
		p.mu.Lock()
		pending := p.pending
		p.pending = make(map[string]chan responseResult)
		if err == nil && len(pending) != 0 {
			err = errors.New("database adapter process exited with pending requests")
		}
		p.waitErr = err
		finalErr := p.waitErr
		p.mu.Unlock()

		for _, ch := range pending {
			ch <- responseResult{err: finalErr}
		}
		_ = p.stdin.Close()
		close(p.done)
	})
}

func (p *Process) removePending(requestID string) {
	p.mu.Lock()
	delete(p.pending, requestID)
	p.mu.Unlock()
}

func newRequestID() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate database adapter request id: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

type boundedDiagnostic struct {
	buf       strings.Builder
	remaining int
	truncated bool
}

func (w *boundedDiagnostic) Write(p []byte) (int, error) {
	original := len(p)
	if w.remaining <= 0 {
		w.truncated = w.truncated || original > 0
		return original, nil
	}
	if len(p) > w.remaining {
		p = p[:w.remaining]
		w.truncated = true
	}
	_, _ = w.buf.Write(p)
	w.remaining -= len(p)
	return original, nil
}

func (w *boundedDiagnostic) String() string {
	text := strings.TrimSpace(w.buf.String())
	if w.truncated {
		if text != "" {
			text += "\n"
		}
		text += "[diagnostic output truncated]"
	}
	return text
}
