package hdc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Target struct {
	client *Client
	key    string
}

type ShellConnection struct{ conn *Connection }

func (s *ShellConnection) ReadAll(ctx context.Context) ([]byte, error) { return s.conn.ReadAll(ctx) }

func (t *Target) transport(ctx context.Context) (*Connection, error) {
	// readiness probe similar to TS implementation
	if t.client.opts.Debug {
		fmt.Printf("[transport] target=%s connect probe begin\n", t.key)
	}
	conn, err := t.client.connection(ctx, t.key)
	if err != nil {
		if t.client.opts.Debug {
			fmt.Printf("[transport] target=%s connect probe failed: %v\n", t.key, err)
		}
		return nil, err
	}
	if err := conn.Send([]byte("shell echo ready\n")); err != nil {
		conn.Close()
		if t.client.opts.Debug {
			fmt.Printf("[transport] target=%s probe send failed: %v\n", t.key, err)
		}
		return nil, err
	}
	if _, err := conn.ReadAll(ctx); err != nil {
		if err != io.EOF {
			conn.Close()
			if t.client.opts.Debug {
				fmt.Printf("[transport] target=%s probe read failed: %v\n", t.key, err)
			}
			return nil, err
		}
	}
	// close probe connection and open a fresh one like TS does
	conn.Close()
	c2, err := t.client.connection(ctx, t.key)
	if err != nil {
		if t.client.opts.Debug {
			fmt.Printf("[transport] target=%s connect after-probe failed: %v\n", t.key, err)
		}
		return nil, err
	}
	if t.client.opts.Debug {
		fmt.Printf("[transport] target=%s ready\n", t.key)
	}
	return c2, nil
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
	if t.client.opts.Debug {
		fmt.Printf("[forward] target=%s %s -> %s send\n", t.key, local, remote)
	}
	if err := conn.Send([]byte("fport " + local + " " + remote)); err != nil {
		if t.client.opts.Debug {
			fmt.Printf("[forward] target=%s send failed: %v\n", t.key, err)
		}
		return err
	}
	b, err := conn.ReadValue(ctx)
	if err != nil {
		// 容错：连接在指令执行后被服务端关闭/复位。查询是否已创建成功
		if t.forwardExists(ctx, local, remote) {
			if t.client.opts.Debug {
				fmt.Printf("[forward] target=%s read failed but exists -> success (%v)\n", t.key, err)
			}
			return nil
		}
		if t.client.opts.Debug {
			fmt.Printf("[forward] target=%s read failed and not exists: %v\n", t.key, err)
		}
		return err
	}
	if !bytes.Contains(b, []byte("OK")) {
		if t.client.opts.Debug {
			fmt.Printf("[forward] target=%s resp not OK: %q\n", t.key, string(b))
		}
		return errors.New(string(b))
	}
	if t.client.opts.Debug {
		fmt.Printf("[forward] target=%s done\n", t.key)
	}
	return nil
}

// forwardExists checks if a forward mapping exists for this target.
func (t *Target) forwardExists(ctx context.Context, local, remote string) bool {
	forwards, err := t.ListForwards(ctx)
	if err != nil {
		return false
	}
	for _, f := range forwards {
		if f.Local == local && f.Remote == remote {
			return true
		}
	}
	return false
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
	if t.client.opts.Debug {
		fmt.Printf("[fport rm] target=%s %s -> %s send\n", t.key, local, remote)
	}
	if err := conn.Send([]byte("fport rm " + local + " " + remote)); err != nil {
		return err
	}
	b, err := conn.ReadValue(ctx)
	if err != nil {
		// 容错：若连接被中止，确认转发已被移除则视为成功
		if !t.forwardExists(ctx, local, remote) {
			if t.client.opts.Debug {
				fmt.Printf("[fport rm] target=%s read failed but removed -> success (%v)\n", t.key, err)
			}
			return nil
		}
		if t.client.opts.Debug {
			fmt.Printf("[fport rm] target=%s read failed and still exists: %v\n", t.key, err)
		}
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
	if t.client.opts.Debug {
		fmt.Printf("[reverse] target=%s %s <- %s send\n", t.key, local, remote)
	}
	if err := conn.Send([]byte("rport " + remote + " " + local)); err != nil {
		return err
	}
	b, err := conn.ReadValue(ctx)
	if err != nil {
		// 容错：若连接被中止，确认反向转发已存在则视为成功
		if t.reverseExists(ctx, remote, local) {
			if t.client.opts.Debug {
				fmt.Printf("[reverse] target=%s read failed but exists -> success (%v)\n", t.key, err)
			}
			return nil
		}
		if t.client.opts.Debug {
			fmt.Printf("[reverse] target=%s read failed and not exists: %v\n", t.key, err)
		}
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

// reverseExists checks if a reverse mapping exists for this target.
func (t *Target) reverseExists(ctx context.Context, remote, local string) bool {
	reverses, err := t.ListReverses(ctx)
	if err != nil {
		return false
	}
	for _, f := range reverses {
		if f.Remote == remote && f.Local == local {
			return true
		}
	}
	return false
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
	// pre-flight info
	wd, _ := os.Getwd()
	info, statErr := os.Stat(local)
	if t.client.opts.Debug {
		fmt.Printf("[hdc file send] target=%s bin=%q wd=%q args=%v\n", t.key, t.client.opts.Bin, wd, args)
	}
	if statErr != nil {
		if t.client.opts.Debug {
			fmt.Printf("[hdc file send] target=%s stat local failed: %v\n", t.key, statErr)
		}
	} else {
		if t.client.opts.Debug {
			fmt.Printf("[hdc file send] target=%s local=%q size=%d mode=%v\n", t.key, local, info.Size(), info.Mode())
		}
	}
	start := time.Now()
	out, err := cmd.CombinedOutput()
	dur := time.Since(start)
	var exitCode int
	if ee, ok := err.(*exec.ExitError); ok {
		exitCode = ee.ExitCode()
	} else if err == nil {
		exitCode = 0
	} else {
		exitCode = -1
	}
	preview := string(out)
	if len(preview) > 400 {
		preview = preview[:400] + "..."
	}
	if t.client.opts.Debug {
		fmt.Printf("[hdc file send] target=%s done code=%d dur=%s outlen=%d preview=%q err=%v\n", t.key, exitCode, dur, len(out), preview, err)
	}
	if err != nil {
		if t.client.opts.Debug {
			fmt.Printf("[hdc file send] target=%s err=%v out=%q\n", t.key, err, string(out))
		}
		return err
	}
	lower := strings.ToLower(string(out))
	if bytes.Contains(out, []byte("fail")) || strings.Contains(lower, "error") {
		return errors.New("Send file failed: " + string(out))
	}
	if t.client.opts.Debug {
		fmt.Printf("[hdc file send] target=%s ok out=%q\n", t.key, string(out))
	}
	return nil
}

func (t *Target) RecvFile(ctx context.Context, remote, local string) error {
	base := t.hdcArgs()
	args := append(base, "file", "recv", remote, local)
	cmd := exec.CommandContext(ctx, t.client.opts.Bin, args...)
	wd, _ := os.Getwd()
	if t.client.opts.Debug {
		fmt.Printf("[hdc file recv] target=%s bin=%q wd=%q args=%v\n", t.key, t.client.opts.Bin, wd, args)
	}
	start := time.Now()
	out, err := cmd.CombinedOutput()
	dur := time.Since(start)
	var exitCode int
	if ee, ok := err.(*exec.ExitError); ok {
		exitCode = ee.ExitCode()
	} else if err == nil {
		exitCode = 0
	} else {
		exitCode = -1
	}
	preview := string(out)
	if len(preview) > 400 {
		preview = preview[:400] + "..."
	}
	if t.client.opts.Debug {
		fmt.Printf("[hdc file recv] target=%s done code=%d dur=%s outlen=%d preview=%q err=%v\n", t.key, exitCode, dur, len(out), preview, err)
	}
	if err != nil {
		if t.client.opts.Debug {
			fmt.Printf("[hdc file recv] target=%s err=%v out=%q\n", t.key, err, string(out))
		}
		return err
	}
	lower := strings.ToLower(string(out))
	if bytes.Contains(out, []byte("fail")) || strings.Contains(lower, "error") {
		return errors.New("Recv file failed: " + string(out))
	}
	if t.client.opts.Debug {
		fmt.Printf("[hdc file recv] target=%s ok out=%q\n", t.key, string(out))
	}
	return nil
}

func (t *Target) Install(ctx context.Context, hap string) error {
	base := t.hdcArgs()
	args := append(base, "install", hap)
	cmd := exec.CommandContext(ctx, t.client.opts.Bin, args...)
	fmt.Printf("[hdc install] target=%s args=%v\n", t.key, args)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("[hdc install] target=%s err=%v out=%q\n", t.key, err, string(out))
		return err
	}
	lower := strings.ToLower(string(out))
	if strings.Contains(lower, "fail") || strings.Contains(lower, "error") {
		return errors.New(string(out))
	}
	fmt.Printf("[hdc install] target=%s ok out=%q\n", t.key, string(out))
	return nil
}

func (t *Target) Uninstall(ctx context.Context, bundle string) error {
	base := t.hdcArgs()
	args := append(base, "uninstall", bundle)
	cmd := exec.CommandContext(ctx, t.client.opts.Bin, args...)
	fmt.Printf("[hdc uninstall] target=%s args=%v\n", t.key, args)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("[hdc uninstall] target=%s err=%v out=%q\n", t.key, err, string(out))
		return err
	}
	lower := strings.ToLower(string(out))
	if strings.Contains(lower, "fail") || strings.Contains(lower, "error") {
		return errors.New("Uninstall bundle failed: " + string(out))
	}
	fmt.Printf("[hdc uninstall] target=%s ok out=%q\n", t.key, string(out))
	return nil
}
