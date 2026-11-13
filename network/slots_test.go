package network

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSlots(t *testing.T) {
	t.Parallel()

	slots := NewSlots(4)

	for i := 0; i < 4; i++ {
		slots.Release() // should do nothing
	}

	for i := 3; i >= 0; i-- {
		closed := slots.Take(context.Background())
		assert.False(t, closed)
	}

	/* START ALPHA CHANGE 2092 -- measure slot blocking with monotonic elapsed time -- */
	started := time.Now()

	go func() {
		time.Sleep(500 * time.Millisecond)
		slots.Release() // return one slot after 500 milliseconds
		time.Sleep(500 * time.Millisecond)
		slots.Release() // return another slot after 1 second
	}()

	closed1 := slots.Take(context.Background())
	firstElapsed := time.Since(started)

	closed2 := slots.Take(context.Background())
	secondElapsed := time.Since(started)

	assert.False(t, closed1)
	assert.GreaterOrEqual(t, firstElapsed, 500*time.Millisecond)
	assert.False(t, closed2)
	assert.GreaterOrEqual(t, secondElapsed, time.Second)
	/* FINISH ALPHA CHANGE 2092 -- measure slot blocking with monotonic elapsed time -- */
}
