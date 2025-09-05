package hdc

import (
	"context"
	"time"
)

type Tracker struct {
	c       *Client
	ctx     context.Context
	cancel  context.CancelFunc
	added   chan string
	removed chan string
	errs    chan error
	last    []string
}

func (c *Client) TrackTargets(ctx context.Context) (*Tracker, error) {
	tctx, cancel := context.WithCancel(ctx)
	tr := &Tracker{
		c:       c,
		ctx:     tctx,
		cancel:  cancel,
		added:   make(chan string, 8),
		removed: make(chan string, 8),
		errs:    make(chan error, 1),
		last:    []string{},
	}
	go tr.loop()
	return tr, nil
}

func (t *Tracker) loop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			cur, err := t.c.ListTargets(t.ctx)
			if err != nil {
				t.sendErr(err)
				continue
			}
			t.diffAndEmit(cur)
			t.last = cur
		}
	}
}

func (t *Tracker) diffAndEmit(cur []string) {
	// additions
	for _, v := range cur {
		if !contains(t.last, v) {
			t.added <- v
		}
	}
	// removals
	for _, v := range t.last {
		if !contains(cur, v) {
			t.removed <- v
		}
	}
}

func contains(arr []string, v string) bool {
	for _, x := range arr {
		if x == v {
			return true
		}
	}
	return false
}

func (t *Tracker) Added() <-chan string   { return t.added }
func (t *Tracker) Removed() <-chan string { return t.removed }
func (t *Tracker) Errors() <-chan error   { return t.errs }
func (t *Tracker) Close()                 { t.cancel() }

func (t *Tracker) sendErr(err error) {
	select {
	case t.errs <- err:
	default:
	}
}
