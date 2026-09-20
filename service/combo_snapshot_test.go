package service

import (
	"sync"
	"testing"

	"github.com/lza6/new-api-Max/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestComboSnapshotBuildAndResolve：快照构建/解析语义（含 nil/空名过滤）。
func TestComboSnapshotBuildAndResolve(t *testing.T) {
	combos := []*model.ChannelCombo{
		{Name: "c1"},
		{Name: "c2"},
		{Name: ""}, // 空名应被过滤
		nil,        // nil 应被过滤
	}
	snap := buildComboSnapshot(combos)
	require.NotNil(t, snap)
	assert.NotNil(t, snap.byName["c1"])
	assert.NotNil(t, snap.byName["c2"])
	_, hasEmpty := snap.byName[""]
	assert.False(t, hasEmpty)

	comboSnapshotPtr.Store(snap)
	require.Same(t, snap.byName["c1"], ResolveComboSnapshot("c1"))
	assert.Nil(t, ResolveComboSnapshot(""))
	assert.Nil(t, ResolveComboSnapshot("missing"))
}

// TestComboSnapshotConcurrentReadsDuringSwap：并发读者在快照原子替换期间
// 必须始终读到完整一致快照（无锁读路径的核心保证）。
func TestComboSnapshotConcurrentReadsDuringSwap(t *testing.T) {
	comboSnapshotPtr.Store(buildComboSnapshot(nil))
	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 写者：持续原子替换（含空快照与 1 个组合的快照交替）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		i := 0
		for {
			select {
			case <-stop:
				return
			default:
			}
			if i%2 == 0 {
				comboSnapshotPtr.Store(buildComboSnapshot([]*model.ChannelCombo{{Name: "c"}}))
			} else {
				comboSnapshotPtr.Store(buildComboSnapshot(nil))
			}
			i++
		}
	}()

	// 读者：并发无锁解析，不得 panic/race；结果只能是 nil 或 "c"。
	for r := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 2000 {
				got := ResolveComboSnapshot("c")
				if got != nil {
					assert.Equal(t, "c", got.Name)
				}
			}
		}()
		_ = r
	}
	close(stop)
	wg.Wait()
	comboSnapshotPtr.Store(buildComboSnapshot(nil))
}
