package hdc

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	uiHeader = "_uitestkit_rpc_message_head_"
	uiTailer = "_uitestkit_rpc_message_tail_"
)

// UiDriver provides minimal uitest RPC capabilities similar to TS version.
type UiDriver struct {
	target        *Target
	driverName    string
	port          int
	conn          *uiRPCConn
	mu            sync.Mutex
	sdkVersion    string
	sdkPath       string
	needEnsureSDK bool
}

func (d *UiDriver) SetNeedEnsureSDK(needEnsureSDK bool) {
	d.needEnsureSDK = needEnsureSDK
}

func (t *Target) CreateUiDriver() *UiDriver { return &UiDriver{target: t} }

// SetSdk allows overriding sdk path and version.
func (d *UiDriver) SetSdk(path, version string) { d.sdkPath = path; d.sdkVersion = version }

func (d *UiDriver) Start(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn != nil {
		return nil
	}
	if d.target.client.opts.Debug {
		fmt.Printf("[ui] start target=%s\n", d.target.key)
	}
	// enable test mode
	if err := d.shell(ctx, "param set persist.ace.testmode.enabled 1"); err != nil {
		if d.target.client.opts.Debug {
			fmt.Printf("[ui] enable test mode failed: %v\n", err)
		}
	}
	// ensure SDK agent
	if err := d.ensureSdk(ctx); err != nil {
		if d.target.client.opts.Debug {
			fmt.Println("[ui] ensureSdk failed", err)
		}
		return err
	}
	// ensure uitest daemon running
	if err := d.shell(ctx, "uitest start-daemon singleness"); err != nil {
		if d.target.client.opts.Debug {
			fmt.Printf("[ui] start-daemon failed: %v\n", err)
		}
	}
	// give daemon time to come up similar to TS (slightly longer for slow devices)
	time.Sleep(3 * time.Second)
	// ensure forward tcp:8012
	p, err := d.forwardTcp(ctx, 8012)
	if err != nil {
		if d.target.client.opts.Debug {
			fmt.Println("[ui] forwardTcp failed", err)
		}
		return err
	}
	rpc := &uiRPCConn{}
	if err := rpc.Connect(ctx, p); err != nil {
		if d.target.client.opts.Debug {
			fmt.Println("[ui] rpc.Connect failed", err)
		}
		return err
	}
	// create driver
	payload := map[string]any{
		"module": "com.ohos.devicetest.hypiumApiHelper",
		"method": "callHypiumApi",
		"params": map[string]any{
			"api":          "Driver.create",
			"this":         nil,
			"args":         []any{},
			"message_type": "hypium",
		},
	}
	if d.target.client.opts.Debug {
		fmt.Printf("[ui] create driver via rpc\n")
	}
	res, err := rpc.SendMessage(ctx, payload, 3*time.Second)
	if err != nil {
		if d.target.client.opts.Debug {
			fmt.Println("[ui] rpc.SendMessage failed", err)
		}
		// Recovery: remove device agent and resend, restart daemon, reconnect, retry once
		rpc.Close()
		// 仅当设备端 agent 缺失或版本过低时才重装
		needReinstall := true
		if raw, e := d.catAgent(ctx); e == nil {
			cur := extractVersion(raw)
			want := d.sdkVersion
			if want == "" {
				want = "1.1.0"
			}
			if strings.Contains(raw, "UITEST_AGENT_LIBRARY") && cmpVersion(cur, want) >= 0 {
				needReinstall = false
			}
			if d.target.client.opts.Debug {
				fmt.Printf("[ui] catAgent ok cur=%q want=%q reinstall=%v\n", cur, want, needReinstall)
			}
		} else {
			if d.target.client.opts.Debug {
				fmt.Printf("[ui] catAgent failed: %v\n", e)
			}
		}
		if needReinstall {
			_ = d.shell(ctx, "rm /data/local/tmp/agent.so")
			if d.sdkPath == "" {
				d.sdkPath = defaultSdkPath()
			}
			// 发送带重试
			var sendErr error
			for i := 0; i < 3; i++ {
				sendErr = d.target.SendFile(ctx, d.sdkPath, "/data/local/tmp/agent.so")
				if sendErr == nil {
					break
				}
				if d.target.client.opts.Debug {
					fmt.Printf("[ui] send agent retry %d failed: %v\n", i+1, sendErr)
				}
				time.Sleep(500 * time.Millisecond)
			}
			if sendErr != nil {
				return fmt.Errorf("reinstall agent failed: %w", sendErr)
			}
		}
		_ = d.shell(ctx, "uitest start-daemon singleness")
		time.Sleep(3 * time.Second)
		rpc = &uiRPCConn{}
		if err2 := rpc.Connect(ctx, p); err2 != nil {
			return err2
		}
		res, err = rpc.SendMessage(ctx, payload, 3*time.Second)
		if err != nil {
			rpc.Close()
			return err
		}
	}
	if s, ok := res.(string); ok {
		d.driverName = s
	} else {
		rpc.Close()
		fmt.Println("invalid create response")
		return errors.New("invalid create response")
	}
	d.conn = rpc
	d.port = p
	return nil
}

func (d *UiDriver) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn != nil {
		d.conn.Close()
		d.conn = nil
	}
	// best-effort kill uitest daemon
	_ = d.shell(context.Background(), "sh -c 'pidof uitest && kill -9 $(pidof uitest)'")
}

func (d *UiDriver) GetDisplaySize(ctx context.Context) (map[string]any, error) {
	if err := d.ensure(ctx); err != nil {
		return nil, err
	}
	res, err := d.call(ctx, "CtrlCmd", "getDisplaySize", nil)
	if err != nil {
		return nil, err
	}
	if m, ok := res.(map[string]any); ok {
		return m, nil
	}
	return nil, errors.New("unexpected result")
}

func (d *UiDriver) InputText(ctx context.Context, text string, x, y int) error {
	if err := d.ensure(ctx); err != nil {
		return err
	}
	_, err := d.conn.SendMessage(ctx, map[string]any{
		"module": "com.ohos.devicetest.hypiumApiHelper",
		"method": "callHypiumApi",
		"params": map[string]any{
			"api":          "Driver.inputText",
			"this":         d.driverName,
			"args":         []any{map[string]int{"x": x, "y": y}, text},
			"message_type": "hypium",
		},
	}, 3*time.Second)
	return err
}

func (d *UiDriver) CaptureLayout(ctx context.Context) (any, error) {
	if err := d.ensure(ctx); err != nil {
		return nil, err
	}
	return d.call(ctx, "Captures", "captureLayout", nil)
}

func (d *UiDriver) ensure(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn != nil {
		return nil
	}
	return d.Start(ctx)
}

func (d *UiDriver) call(ctx context.Context, method, api string, args any) (any, error) {
	payload := map[string]any{
		"module": "com.ohos.devicetest.hypiumApiHelper",
		"method": method,
		"params": map[string]any{
			"api":  api,
			"args": args,
		},
	}
	return d.conn.SendMessage(ctx, payload, 3*time.Second)
}

func (d *UiDriver) forwardTcp(ctx context.Context, remotePort int) (int, error) {
	remote := "tcp:" + strconv.Itoa(remotePort)
	forwards, err := d.target.ListForwards(ctx)
	if err == nil {
		for _, f := range forwards {
			if f.Remote == remote {
				// parse local tcp:port
				if strings.HasPrefix(f.Local, "tcp:") {
					if p, err := strconv.Atoi(strings.TrimPrefix(f.Local, "tcp:")); err == nil {
						return p, nil
					}
				}
			}
		}
	}
	// allocate free port
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	p := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	local := "tcp:" + strconv.Itoa(p)
	if err := d.target.Forward(ctx, local, remote); err != nil {
		if err == io.EOF {
			return p, nil
		}
		return 0, err
	}
	return p, nil
}

func (d *UiDriver) shell(ctx context.Context, cmd string) error {
	c, err := d.target.Shell(ctx, cmd)
	if err != nil {
		return err
	}
	_, err = c.ReadAll(ctx)
	return err
}

// ensureSdk checks and pushes uitest agent if needed.
func (d *UiDriver) ensureSdk(ctx context.Context) error {
	if !d.needEnsureSDK {
		return nil
	}
	if d.sdkVersion == "" {
		d.sdkVersion = "1.1.0"
	}
	if d.sdkPath == "" {
		d.sdkPath = defaultSdkPath()
	}
	// check version on device
	raw, _ := d.catAgent(ctx)
	if !strings.Contains(raw, "UITEST_AGENT_LIBRARY") || cmpVersion(extractVersion(raw), d.sdkVersion) < 0 {
		_ = d.shell(ctx, "rm /data/local/tmp/agent.so")
		if err := d.target.SendFile(ctx, d.sdkPath, "/data/local/tmp/agent.so"); err != nil {
			return fmt.Errorf("send agent failed: %w", err)
		}
	}
	return nil
}

func (d *UiDriver) catAgent(ctx context.Context) (string, error) {
	c, err := d.target.Shell(ctx, "cat /data/local/tmp/agent.so | grep -a UITEST_AGENT_LIBRARY")
	if err != nil {
		return "", err
	}
	b, err := c.ReadAll(ctx)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func defaultSdkPath() string {
	wd, _ := os.Getwd()
	candidates := []string{
		filepath.Join(wd, "uitestkit_sdk", "uitest_agent_v1.1.0.so"),
		filepath.Join(wd, "..", "uitestkit_sdk", "uitest_agent_v1.1.0.so"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "uitestkit_sdk/uitest_agent_v1.1.0.so"
}

// minimal version compare: returns -1/0/1
func cmpVersion(a, b string) int {
	if a == b {
		return 0
	}
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		ai, bi := 0, 0
		if i < len(as) {
			ai, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			bi, _ = strconv.Atoi(bs[i])
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}

func extractVersion(raw string) string {
	idx := strings.Index(raw, "@v")
	if idx >= 0 {
		return strings.TrimSpace(raw[idx+2:])
	}
	return ""
}

// uiRPCConn implements uitest RPC framing protocol.
type uiRPCConn struct {
	c        net.Conn
	mu       sync.Mutex
	resolves map[uint32]chan any
	onMsg    func(session uint32, payload []byte)
}

func (u *uiRPCConn) Connect(ctx context.Context, port int) error {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return err
	}
	u.c = conn
	u.resolves = make(map[uint32]chan any)
	go u.readLoop()
	return nil
}

func (u *uiRPCConn) Close() {
	if u.c != nil {
		_ = u.c.Close()
	}
}

func (u *uiRPCConn) OnMessage(cb func(session uint32, payload []byte)) {
	u.mu.Lock()
	u.onMsg = cb
	u.mu.Unlock()
}

func (u *uiRPCConn) SendMessage(ctx context.Context, message any, timeout time.Duration) (any, error) {
	payload, _ := json.Marshal(message)
	sessionId := uint32(time.Now().UnixNano())
	sid := make([]byte, 4)
	binary.BigEndian.PutUint32(sid, sessionId)
	// frame: header + sessionId + len + payload + tailer
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(payload)))
	frame := make([]byte, 0, len(uiHeader)+8+len(payload)+len(uiTailer))
	frame = append(frame, []byte(uiHeader)...)
	frame = append(frame, sid...)
	frame = append(frame, length[:]...)
	frame = append(frame, payload...)
	frame = append(frame, []byte(uiTailer)...)

	ch := make(chan any, 1)
	u.mu.Lock()
	u.resolves[sessionId] = ch
	u.mu.Unlock()

	if _, err := u.c.Write(frame); err != nil {
		return nil, err
	}

	if timeout > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case resp := <-ch:
			return resp, nil
		case <-time.After(timeout):
			u.mu.Lock()
			delete(u.resolves, sessionId)
			u.mu.Unlock()
			return nil, errors.New("timeout")
		}
	}
	return <-ch, nil
}

// SendMessageWithSession sends and returns (sessionId, result, error).
func (u *uiRPCConn) SendMessageWithSession(ctx context.Context, message any, timeout time.Duration) (uint32, any, error) {
	payload, _ := json.Marshal(message)
	sessionId := uint32(time.Now().UnixNano())
	sid := make([]byte, 4)
	binary.BigEndian.PutUint32(sid, sessionId)
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(payload)))
	frame := make([]byte, 0, len(uiHeader)+8+len(payload)+len(uiTailer))
	frame = append(frame, []byte(uiHeader)...)
	frame = append(frame, sid...)
	frame = append(frame, length[:]...)
	frame = append(frame, payload...)
	frame = append(frame, []byte(uiTailer)...)

	ch := make(chan any, 1)
	u.mu.Lock()
	u.resolves[sessionId] = ch
	u.mu.Unlock()

	if _, err := u.c.Write(frame); err != nil {
		return sessionId, nil, err
	}
	var resp any
	if timeout > 0 {
		select {
		case <-ctx.Done():
			return sessionId, nil, ctx.Err()
		case r := <-ch:
			resp = r
		case <-time.After(timeout):
			u.mu.Lock()
			delete(u.resolves, sessionId)
			u.mu.Unlock()
			return sessionId, nil, errors.New("timeout")
		}
	} else {
		resp = <-ch
	}
	return sessionId, resp, nil
}

func (u *uiRPCConn) readLoop() {
	header := []byte(uiHeader)
	tailer := []byte(uiTailer)
	buf := make([]byte, 0)
	tmp := make([]byte, 4096)
	for {
		n, err := u.c.Read(tmp)
		if err != nil {
			return
		}
		buf = append(buf, tmp[:n]...)
		for {
			if len(buf) < len(header)+8 {
				break
			}
			if string(buf[:len(header)]) != uiHeader {
				buf = buf[:0]
				break
			}
			sid := binary.BigEndian.Uint32(buf[len(header):])
			l := binary.BigEndian.Uint32(buf[len(header)+4:])
			total := len(header) + 8 + int(l) + len(tailer)
			if len(buf) < total {
				break
			}
			start := len(header) + 8
			end := start + int(l)
			payload := buf[start:end]
			if string(buf[end:total]) != uiTailer {
				buf = buf[:0]
				break
			}
			// try JSON first
			var result struct {
				Result    any `json:"result"`
				Exception *struct {
					Message string `json:"message"`
				} `json:"exception"`
			}
			var val any
			if err := json.Unmarshal(payload, &result); err == nil {
				if result.Exception != nil {
					val = errors.New(result.Exception.Message)
				} else {
					val = result.Result
				}
			} else {
				val = payload
			}
			u.mu.Lock()
			ch := u.resolves[sid]
			delete(u.resolves, sid)
			cb := u.onMsg
			u.mu.Unlock()
			if ch != nil {
				ch <- val
			} else if cb != nil {
				cb(sid, payload)
			}
			buf = buf[total:]
		}
	}
}
