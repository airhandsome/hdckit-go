package hdc

import (
	"context"
)

func (d *UiDriver) TouchDown(ctx context.Context, x, y int) error {
	if err := d.ensure(ctx); err != nil {
		return err
	}
	_, err := d.call(ctx, "Gestures", "touchDown", map[string]int{"x": x, "y": y})
	return err
}

func (d *UiDriver) TouchMove(ctx context.Context, x, y int) error {
	if err := d.ensure(ctx); err != nil {
		return err
	}
	_, err := d.call(ctx, "Gestures", "touchMove", map[string]int{"x": x, "y": y})
	return err
}

func (d *UiDriver) TouchUp(ctx context.Context, x, y int) error {
	if err := d.ensure(ctx); err != nil {
		return err
	}
	_, err := d.call(ctx, "Gestures", "touchUp", map[string]int{"x": x, "y": y})
	return err
}
