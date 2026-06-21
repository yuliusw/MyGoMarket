package lock

import (
	"fmt"
	"sync/atomic"
)

// snapshot 保存数据和版本的快照，一旦创建就不再修改（Immutable）
type snapshot[T any] struct {
	data    T
	version int64
}

// Versioned 包装类
type Versioned[T any] struct {
	value atomic.Value // 存储 *snapshot[T]
}

// Pack 包装初始对象
func Pack[T any](data T) *Versioned[T] {
	v := &Versioned[T]{}
	v.value.Store(&snapshot[T]{data: data, version: 1})
	return v
}

// Get 获取当前快照（无锁读取）
func (v *Versioned[T]) Get() (T, int64) {
	s := v.value.Load().(*snapshot[T])
	return s.data, s.version
}

// TryUpdate 乐观更新尝试
func (v *Versioned[T]) TryUpdate(oldVersion int64, fn func(T) (T, error)) error {
	// 1. 获取当前最新状态
	current := v.value.Load().(*snapshot[T])

	// 2. 校验版本：如果传入的版本已经过期，直接失败
	if current.version != oldVersion {
		return fmt.Errorf("version mismatch: expected %d, got %d", oldVersion, current.version)
	}

	// 3. 计算新值
	newData, err := fn(current.data)
	if err != nil {
		return err
	}

	// 4. 构建新快照
	newSnapshot := &snapshot[T]{
		data:    newData,
		version: current.version + 1,
	}

	// 5. CAS 操作：只有当前快照还是刚才那个，才切换过去
	if swapped := v.value.CompareAndSwap(current, newSnapshot); !swapped {
		return fmt.Errorf("concurrent modification detected")
	}

	return nil
}

// UpdateWithRetry 自动重试
func (v *Versioned[T]) UpdateWithRetry(maxRetries int, fn func(T) (T, error)) error {
	for i := 0; i < maxRetries; i++ {
		// 每次重试都重新“读”最新的版本
		_, currVer := v.Get()
		err := v.TryUpdate(currVer, fn)
		if err == nil {
			return nil
		}
		// 如果是业务逻辑报错，直接中断重试
		// 这里可以根据 error 类型判断是否是冲突导致
	}
	return fmt.Errorf("failed after %d retries", maxRetries)
}
