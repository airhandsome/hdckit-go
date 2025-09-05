package hdc

import (
	"context"
	"errors"
)

// StartCaptureScreen starts screen stream; callback receives raw frame payloads.
func (d *UiDriver) StartCaptureScreen(ctx context.Context, cb func([]byte), scale float64) (int, error) {
	if scale <= 0 || scale >= 1 {
		scale = 0
	}
	if err := d.ensure(ctx); err != nil {
		return 0, err
	}
	opts := map[string]any{}
	if scale > 0 {
		opts["scale"] = scale
	}
	// call startCaptureScreen, returns sessionId
	res, err := d.call(ctx, "Captures", "startCaptureScreen", map[string]any{"options": opts})
	if err != nil {
		return 0, err
	}
	m, ok := res.(map[string]any)
	if !ok {
		return 0, errors.New("unexpected startCaptureScreen result")
	}
	sidVal, ok := m["sessionId"]
	if !ok {
		return 0, errors.New("missing sessionId")
	}
	sid, ok := toInt(sidVal)
	if !ok {
		return 0, errors.New("invalid sessionId")
	}

	// attach message handler
	d.conn.OnMessage(func(session uint32, payload []byte) {
		if int(session) == sid {
			cb(payload)
		}
	})
	return sid, nil
}

func (d *UiDriver) StopCaptureScreen(ctx context.Context) error {
	if err := d.ensure(ctx); err != nil {
		return err
	}
	_, err := d.call(ctx, "Captures", "stopCaptureScreen", nil)
	d.conn.OnMessage(nil)
	return err
}

func toInt(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		return int(x), true
	case int:
		return x, true
	default:
		return 0, false
	}
}
