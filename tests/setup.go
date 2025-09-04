package tests

import (
	"context"
	"testing"
	"time"
)

type TestSuite struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func (ts *TestSuite) Setup(t *testing.T) {
	ts.ctx, ts.cancel = context.WithTimeout(context.Background(), 1*time.Minute)
	t.Cleanup(ts.cancel)
}
