package hdc

import (
	"context"
	"os"
	"os/exec"
	"strconv"
)

type Options struct {
	Host  string
	Port  int
	Bin   string
	Debug bool
}

type Client struct {
	opts Options
}

func NewClient(o Options) *Client {
	if o.Host == "" {
		o.Host = "127.0.0.1"
	}
	if o.Port == 0 {
		if p := os.Getenv("OHOS_HDC_SERVER_PORT"); p != "" {
			// ignore error, keep zero on parse failure
			if v, err := strconv.Atoi(p); err == nil {
				o.Port = v
			}
		}
		if o.Port == 0 {
			o.Port = 8710
		}
	}
	if o.Bin == "" {
		o.Bin = "hdc"
	}
	return &Client{opts: o}
}

func (c *Client) connection(ctx context.Context, connectKey string) (*Connection, error) {
	conn := NewConnection(c.opts)
	if err := conn.Connect(ctx, connectKey); err != nil {
		if !conn.triedStart {
			_ = c.startServer(ctx)
			conn.triedStart = true
			if e2 := conn.Connect(ctx, connectKey); e2 == nil {
				return conn, nil
			} else {
				return nil, e2
			}
		}
		return nil, err
	}
	return conn, nil
}

func (c *Client) startServer(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, c.opts.Bin, "start")
	cmd.Env = append(os.Environ(),
		"OHOS_HDC_SERVER_PORT="+strconv.Itoa(c.opts.Port),
	)
	return cmd.Run()
}

func (c *Client) ListTargets(ctx context.Context) ([]string, error) {
	conn, err := c.connection(ctx, "")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.Send([]byte("list targets")); err != nil {
		return nil, err
	}
	b, err := conn.ReadValue(ctx)
	if err != nil {
		return nil, err
	}
	return readTargets(string(b)), nil
}

func (c *Client) Target(connectKey string) *Target { return &Target{client: c, key: connectKey} }

func (c *Client) ListForwards(ctx context.Context) ([]Forward, error) {
	conn, err := c.connection(ctx, "")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.Send([]byte("fport ls")); err != nil {
		return nil, err
	}
	b, err := conn.ReadValue(ctx)
	if err != nil {
		return nil, err
	}
	return readPorts(string(b), false), nil
}

func (c *Client) ListReverses(ctx context.Context) ([]Forward, error) {
	conn, err := c.connection(ctx, "")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.Send([]byte("fport ls")); err != nil {
		return nil, err
	}
	b, err := conn.ReadValue(ctx)
	if err != nil {
		return nil, err
	}
	return readPorts(string(b), true), nil
}

// Kill kills last known hdc server process recorded by hdc itself.
func (c *Client) Kill() {
	if pid := getLastPid(); pid > 0 {
		p, err := os.FindProcess(pid)
		if err == nil {
			_ = p.Kill()
		}
	}
}
