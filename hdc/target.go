package hdc

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

type Target struct {
	client *Client
	key    string
}

type ShellConnection struct{ conn *Connection }

func (s *ShellConnection) ReadAll(ctx context.Context) ([]byte, error) { return s.conn.ReadAll(ctx) }

func (t *Target) transport(ctx context.Context) (*Connection, error) {
	// readiness probe similar to TS implementation
	conn, err := t.client.connection(ctx, t.key)
	if err != nil {
		return nil, err
	}
	if err := conn.Send([]byte("shell echo ready\n")); err != nil {
		conn.Close()
		return nil, err
	}
	if _, err := conn.ReadAll(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	return t.client.connection(ctx, t.key)
}

func (t *Target) GetParameters(ctx context.Context) (map[string]string, error) {
	conn, err := t.transport(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.Send([]byte("shell param get")); err != nil {
		return nil, err
	}
	b, err := conn.ReadAll(ctx)
	if err != nil {
		return nil, err
	}
	return parseParameters(string(b)), nil
}

func (t *Target) Shell(ctx context.Context, command string) (*ShellConnection, error) {
	conn, err := t.transport(ctx)
	if err != nil {
		return nil, err
	}
	if err := conn.Send([]byte("shell " + command)); err != nil {
		conn.Close()
		return nil, err
	}
	return &ShellConnection{conn: conn}, nil
}

func (t *Target) Forward(ctx context.Context, local, remote string) error {
	conn, err := t.transport(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.Send([]byte("fport " + local + " " + remote)); err != nil {
		return err
	}
	b, err := conn.ReadValue(ctx)
	if err != nil {
		return err
	}
	if !bytes.Contains(b, []byte("OK")) {
		return errors.New(string(b))
	}
	return nil
}

func (t *Target) ListForwards(ctx context.Context) ([]Forward, error) {
	all, err := t.client.ListForwards(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Forward, 0)
	for _, f := range all {
		if f.Target == t.key {
			out = append(out, f)
		}
	}
	return out, nil
}

func (t *Target) RemoveForward(ctx context.Context, local, remote string) error {
	conn, err := t.transport(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.Send([]byte("fport rm " + local + " " + remote)); err != nil {
		return err
	}
	b, err := conn.ReadValue(ctx)
	if err != nil {
		return err
	}
	if !bytes.Contains(b, []byte("success")) {
		return errors.New(string(b))
	}
	return nil
}

func (t *Target) Reverse(ctx context.Context, remote, local string) error {
	conn, err := t.transport(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.Send([]byte("rport " + remote + " " + local)); err != nil {
		return err
	}
	b, err := conn.ReadValue(ctx)
	if err != nil {
		return err
	}
	if !bytes.Contains(b, []byte("OK")) {
		return errors.New(string(b))
	}
	return nil
}

func (t *Target) ListReverses(ctx context.Context) ([]Forward, error) {
	all, err := t.client.ListReverses(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Forward, 0)
	for _, f := range all {
		if f.Target == t.key {
			out = append(out, f)
		}
	}
	return out, nil
}

func (t *Target) RemoveReverse(ctx context.Context, remote, local string) error {
	return t.RemoveForward(ctx, local, remote)
}

// hdcArgs builds base args with optional -s host:port and the device -t key.
func (t *Target) hdcArgs() []string {
	args := []string{}
	h := t.client.opts.Host
	p := t.client.opts.Port
	if h != "" || p != 0 {
		addr := h
		if addr == "" {
			addr = "127.0.0.1"
		}
		if p != 0 {
			addr = addr + ":" + itoa(p)
		}
		args = append(args, "-s", addr)
	}
	args = append(args, "-t", t.key)
	return args
}

func (t *Target) SendFile(ctx context.Context, local, remote string) error {
	base := t.hdcArgs()
	args := append(base, "file", "send", local, remote)
	cmd := exec.CommandContext(ctx, t.client.opts.Bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	if !bytes.Contains(out, []byte("finish")) {
		return errors.New("Send file failed: " + string(out))
	}
	return nil
}

func (t *Target) RecvFile(ctx context.Context, remote, local string) error {
	base := t.hdcArgs()
	args := append(base, "file", "recv", remote, local)
	cmd := exec.CommandContext(ctx, t.client.opts.Bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	if !bytes.Contains(out, []byte("finish")) {
		return errors.New("Recv file failed: " + string(out))
	}
	return nil
}

func (t *Target) Install(ctx context.Context, hap string) error {
	base := t.hdcArgs()
	args := append(base, "install", hap)
	cmd := exec.CommandContext(ctx, t.client.opts.Bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	if !bytes.Contains(out, []byte("install bundle successfully")) {
		return errors.New(string(out))
	}
	return nil
}

func (t *Target) Uninstall(ctx context.Context, bundle string) error {
	base := t.hdcArgs()
	args := append(base, "uninstall", bundle)
	cmd := exec.CommandContext(ctx, t.client.opts.Bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	if !bytes.Contains(out, []byte("uninstall bundle successfully")) {
		return errors.New("Uninstall bundle failed")
	}
	return nil
}
