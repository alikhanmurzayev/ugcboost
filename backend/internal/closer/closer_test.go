package closer

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
)

func TestCloser_Close(t *testing.T) {
	t.Parallel()

	t.Run("LIFO order", func(t *testing.T) {
		t.Parallel()
		var order []string
		log := logmocks.NewMockLogger(t)
		c := New(log)

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

		log.EXPECT().Info(mock.Anything, "shutting down", []any{"resource", "third"}).Once()
		log.EXPECT().Info(mock.Anything, "shutting down", []any{"resource", "second"}).Once()
		log.EXPECT().Info(mock.Anything, "shutting down", []any{"resource", "first"}).Once()

		err := c.Close(context.Background())
		require.NoError(t, err)
		require.Equal(t, []string{"third", "second", "first"}, order)
	})

	t.Run("all called on error", func(t *testing.T) {
		t.Parallel()
		var called []string
		log := logmocks.NewMockLogger(t)
		c := New(log)

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

		log.EXPECT().Info(mock.Anything, "shutting down", []any{"resource", "c"}).Once()
		log.EXPECT().Info(mock.Anything, "shutting down", []any{"resource", "b"}).Once()
		log.EXPECT().Info(mock.Anything, "shutting down", []any{"resource", "a"}).Once()
		log.EXPECT().Error(mock.Anything, "shutdown error", mock.MatchedBy(func(args []any) bool {
			return len(args) == 4 && args[0] == "resource" && args[1] == "b" && args[2] == "error"
		})).Once()

		err := c.Close(context.Background())
		require.Error(t, err)
		require.Equal(t, "b failed", err.Error())
		require.Equal(t, []string{"c", "b", "a"}, called, "all functions should be called even on error")
	})

	t.Run("returns first error", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		c := New(log)

		c.Add("a", func(_ context.Context) error { return errors.New("err-a") })
		c.Add("b", func(_ context.Context) error { return errors.New("err-b") })

		log.EXPECT().Info(mock.Anything, "shutting down", []any{"resource", "b"}).Once()
		log.EXPECT().Info(mock.Anything, "shutting down", []any{"resource", "a"}).Once()
		log.EXPECT().Error(mock.Anything, "shutdown error", mock.MatchedBy(func(args []any) bool {
			return len(args) == 4 && args[0] == "resource" && args[1] == "b" && args[2] == "error"
		})).Once()
		log.EXPECT().Error(mock.Anything, "shutdown error", mock.MatchedBy(func(args []any) bool {
			return len(args) == 4 && args[0] == "resource" && args[1] == "a" && args[2] == "error"
		})).Once()

		err := c.Close(context.Background())
		require.Equal(t, "err-b", err.Error(), "should return first error encountered (LIFO order)")
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		c := New(log)

		err := c.Close(context.Background())
		require.NoError(t, err)
	})

	t.Run("context passed", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		c := New(log)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		var receivedCtx context.Context
		c.Add("test", func(ctx context.Context) error {
			receivedCtx = ctx
			return nil
		})

		log.EXPECT().Info(mock.Anything, "shutting down", []any{"resource", "test"}).Once()

		_ = c.Close(ctx)
		require.Equal(t, ctx, receivedCtx)
	})
}
