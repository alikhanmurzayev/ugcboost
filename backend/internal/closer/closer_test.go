package closer

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCloser_Close(t *testing.T) {
	t.Parallel()

	t.Run("LIFO order", func(t *testing.T) {
		t.Parallel()
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
		require.Equal(t, []string{"third", "second", "first"}, order)
	})

	t.Run("all called on error", func(t *testing.T) {
		t.Parallel()
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
		require.Error(t, err)
		require.Equal(t, "b failed", err.Error())
		require.Equal(t, []string{"c", "b", "a"}, called, "all functions should be called even on error")
	})

	t.Run("returns first error", func(t *testing.T) {
		t.Parallel()
		c := New()

		c.Add("a", func(_ context.Context) error { return errors.New("err-a") })
		c.Add("b", func(_ context.Context) error { return errors.New("err-b") })

		err := c.Close(context.Background())
		require.Equal(t, "err-b", err.Error(), "should return first error encountered (LIFO order)")
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		c := New()
		err := c.Close(context.Background())
		require.NoError(t, err)
	})

	t.Run("context passed", func(t *testing.T) {
		t.Parallel()
		c := New()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		var receivedCtx context.Context
		c.Add("test", func(ctx context.Context) error {
			receivedCtx = ctx
			return nil
		})

		_ = c.Close(ctx)
		require.Equal(t, ctx, receivedCtx)
	})
}
