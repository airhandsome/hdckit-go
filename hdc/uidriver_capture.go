package hdc

import (
	"context"
	"encoding/json"
	"errors"
	"time"
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
	// Prefer protocol sessionId (robust even when result is boolean)
	payload := map[string]any{
		"module": "com.ohos.devicetest.hypiumApiHelper",
		"method": "Captures",
		"params": map[string]any{
			"api":  "startCaptureScreen",
			"args": map[string]any{"options": opts},
		},
	}
	sidU32, _, err := d.conn.SendMessageWithSession(ctx, payload, 3*time.Second)
	if err == nil {
		sid := int(sidU32)
		attachCaptureHandler(d, sid, cb)
		return sid, nil
	}
	// Fallback to generic call parsing
	res2, err2 := d.call(ctx, "Captures", "startCaptureScreen", map[string]any{"options": opts})
	if err2 != nil {
		return 0, err2
	}
	// direct number
	if sid, ok := toInt(res2); ok {
		attachCaptureHandler(d, sid, cb)
		return sid, nil
	}
	// map or nested map
	if sid, ok := getSessionIdFromAny(res2); ok {
		attachCaptureHandler(d, sid, cb)
		return sid, nil
	}
	// []byte json
	if b, ok := res2.([]byte); ok {
		var m map[string]any
		if err := json.Unmarshal(b, &m); err == nil {
			if sid, ok := getSessionIdFromAny(m); ok {
				attachCaptureHandler(d, sid, cb)
				return sid, nil
			}
		}
	}
	return 0, errors.New("unexpected startCaptureScreen result")
}

func attachCaptureHandler(d *UiDriver, sid int, cb func([]byte)) {
	d.conn.OnMessage(func(session uint32, payload []byte) {
		if int(session) == sid {
			cb(payload)
		}
	})
}

func getSessionIdFromAny(v any) (int, bool) {
	if m, ok := v.(map[string]any); ok {
		// { sessionId }
		if sid, ok := toInt(m["sessionId"]); ok {
			return sid, true
		}
		// { result: number | { sessionId } }
		if inner, ok := m["result"]; ok {
			if sid, ok := toInt(inner); ok {
				return sid, true
			}
			if mm, ok := inner.(map[string]any); ok {
				if sid, ok := toInt(mm["sessionId"]); ok {
					return sid, true
				}
			}
		}
	}
	return 0, false
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
