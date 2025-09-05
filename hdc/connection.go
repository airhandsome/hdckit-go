package hdc

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"strconv"
	"time"
)

const handshakePrefix = "OHOS HDC"

type Connection struct {
	c          net.Conn
	opts       Options
	ended      bool
	triedStart bool
}

func NewConnection(o Options) *Connection { return &Connection{opts: o} }

func (c *Connection) Connect(ctx context.Context, connectKey string) error {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(c.opts.Host, itoa(c.opts.Port)))
	if err != nil {
		return err
	}
	c.c = conn
	// handshake
	banner, err := c.ReadValue(ctx)
	if err != nil {
		conn.Close()
		return err
	}
	if len(banner) < len(handshakePrefix) || string(banner[:len(handshakePrefix)]) != handshakePrefix {
		conn.Close()
		return errors.New("Channel Hello failed")
	}
	// send back handshake: banner + connectKey(32 bytes)
	key := make([]byte, 32)
	copy(key, []byte(connectKey))
	data := append(banner, key...)
	if err := c.Send(data); err != nil {
		conn.Close()
		return err
	}
	return nil
}

func (c *Connection) Close() {
	if c.c != nil {
		c.c.Close()
		c.c = nil
		c.ended = true
	}
}

func (c *Connection) Send(payload []byte) error {
	if c.c == nil {
		return errors.New("no conn")
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(payload)))
	_, err := c.c.Write(append(hdr[:], payload...))
	return err
}

func (c *Connection) ReadBytes(ctx context.Context, n int) ([]byte, error) {
	if c.c == nil {
		return nil, errors.New("no conn")
	}
	buf := make([]byte, n)
	_, err := ioReadFull(ctx, c.c, buf)
	return buf, err
}

func (c *Connection) ReadValue(ctx context.Context) ([]byte, error) {
	b, err := c.ReadBytes(ctx, 4)
	if err != nil {
		return nil, err
	}
	l := binary.BigEndian.Uint32(b)
	if l == 0 {
		return []byte{}, nil
	}
	return c.ReadBytes(ctx, int(l))
}

func (c *Connection) ReadAll(ctx context.Context) ([]byte, error) {
	var all []byte
	for {
		v, err := c.ReadValue(ctx)
		if err != nil {
			if c.ended {
				return all, nil
			}
			return nil, err
		}
		all = append(all, v...)
		// non-blocking hint; relies on remote end closing stream when finished
		c.c.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
		if _, err := c.c.Read(make([]byte, 0)); err != nil {
			// deadline or end
			c.c.SetReadDeadline(time.Time{})
			return all, nil
		}
		c.c.SetReadDeadline(time.Time{})
	}
}

func itoa(i int) string { return strconv.FormatInt(int64(i), 10) }

// ioReadFull reads exactly len(buf) bytes, honoring context cancellation.
func ioReadFull(ctx context.Context, r net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		r.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, err := r.Read(buf[total:])
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			select {
			case <-ctx.Done():
				return total, ctx.Err()
			default:
				continue
			}
		}
		if err != nil {
			return total, err
		}
		total += n
	}
	r.SetReadDeadline(time.Time{})
	return total, nil
}
