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

func (ts *TestSuite) Setup(t *testing.T, testTimeout time.Duration) {
	ts.ctx, ts.cancel = context.WithTimeout(context.Background(), testTimeout)
	t.Cleanup(ts.cancel)
}
