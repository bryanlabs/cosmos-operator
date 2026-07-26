package controllers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The controllers package had no tests. These cover the reconcile cadence knob, including the
// guard that stops a bad value turning into a hot reconcile loop.
func TestWithReconcilePeriod(t *testing.T) {
	t.Run("defaults to the historical cadence", func(t *testing.T) {
		r := NewFullNode(nil, nil, nil, nil)
		require.Equal(t, defaultReconcilePeriod, r.reconcilePeriod)
		require.Equal(t, 60*time.Second, r.reconcilePeriod)
	})

	t.Run("applies a supplied period", func(t *testing.T) {
		r := NewFullNode(nil, nil, nil, nil, WithReconcilePeriod(5*time.Minute))
		require.Equal(t, 5*time.Minute, r.reconcilePeriod)
	})

	for _, bad := range []time.Duration{0, -1 * time.Second} {
		t.Run("ignores non-positive period "+bad.String(), func(t *testing.T) {
			r := NewFullNode(nil, nil, nil, nil, WithReconcilePeriod(bad))
			require.Equal(t, defaultReconcilePeriod, r.reconcilePeriod,
				"a non-positive period must not be applied, it would requeue immediately forever")
		})
	}

	t.Run("later options win", func(t *testing.T) {
		r := NewFullNode(nil, nil, nil, nil,
			WithReconcilePeriod(10*time.Second),
			WithReconcilePeriod(30*time.Second),
		)
		require.Equal(t, 30*time.Second, r.reconcilePeriod)
	})
}
