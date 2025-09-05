package hdc

import (
	"context"
)

type HilogConnection struct{ conn *Connection }

// ReadAll streams until remote closes; for continuous logs, caller should read from conn.c directly.
func (h *HilogConnection) ReadAll(ctx context.Context) ([]byte, error) { return h.conn.ReadAll(ctx) }

// OpenHilog opens hilog shell and returns underlying connection.
func (t *Target) OpenHilog(ctx context.Context, clear bool) (*HilogConnection, error) {
	if clear {
		c, err := t.Shell(ctx, "hilog -r")
		if err == nil {
			_, _ = c.ReadAll(ctx)
		}
	}
	conn, err := t.transport(ctx)
	if err != nil {
		return nil, err
	}
	if err := conn.Send([]byte("shell hilog")); err != nil {
		conn.Close()
		return nil, err
	}
	return &HilogConnection{conn: conn}, nil
}
