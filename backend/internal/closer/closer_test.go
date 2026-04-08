package closer

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloser_LIFOOrder(t *testing.T) {
	var order []string
	c := New()

	c.Add("first", func(_ context.Context) error {
		order = append(order, "first")
		return nil
	})
	c.Add("second", func(_ context.Context) error {
		order = append(order, "second")
		return nil
	})
	c.Add("third", func(_ context.Context) error {
		order = append(order, "third")
		return nil
	})

	err := c.Close(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"third", "second", "first"}, order)
}

func TestCloser_AllCalledOnError(t *testing.T) {
	var called []string
	c := New()

	c.Add("a", func(_ context.Context) error {
		called = append(called, "a")
		return nil
	})
	c.Add("b", func(_ context.Context) error {
		called = append(called, "b")
		return errors.New("b failed")
	})
	c.Add("c", func(_ context.Context) error {
		called = append(called, "c")
		return nil
	})

	err := c.Close(context.Background())
	assert.Error(t, err)
	assert.Equal(t, "b failed", err.Error())
	assert.Equal(t, []string{"c", "b", "a"}, called, "all functions should be called even on error")
}

func TestCloser_ReturnsFirstError(t *testing.T) {
	c := New()

	c.Add("a", func(_ context.Context) error { return errors.New("err-a") })
	c.Add("b", func(_ context.Context) error { return errors.New("err-b") })

	err := c.Close(context.Background())
	assert.Equal(t, "err-b", err.Error(), "should return first error encountered (LIFO order)")
}

func TestCloser_Empty(t *testing.T) {
	c := New()
	err := c.Close(context.Background())
	assert.NoError(t, err)
}

func TestCloser_ContextPassed(t *testing.T) {
	c := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var receivedCtx context.Context
	c.Add("test", func(ctx context.Context) error {
		receivedCtx = ctx
		return nil
	})

	_ = c.Close(ctx)
	assert.Equal(t, ctx, receivedCtx)
}
