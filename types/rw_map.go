package types

import (
	"sync"

	"github.com/QuantumNous/new-api/common"
)

type RWMap[K comparable, V any] struct {
	data  map[K]V
	mutex sync.RWMutex
}

func (m *RWMap[K, V]) UnmarshalJSON(b []byte) error {
	return m.replaceFromJSON(b)
}

func (m *RWMap[K, V]) MarshalJSON() ([]byte, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return common.Marshal(m.data)
}

func NewRWMap[K comparable, V any]() *RWMap[K, V] {
	return &RWMap[K, V]{
		data: make(map[K]V),
	}
}

func (m *RWMap[K, V]) Get(key K) (V, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	value, exists := m.data[key]
	return value, exists
}

func (m *RWMap[K, V]) Set(key K, value V) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.data[key] = value
}

func (m *RWMap[K, V]) AddAll(other map[K]V) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for k, v := range other {
		m.data[k] = v
	}
}

func (m *RWMap[K, V]) Clear() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.data = make(map[K]V)
}

func (m *RWMap[K, V]) ReadAll() map[K]V {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	copiedMap := make(map[K]V)
	for k, v := range m.data {
		copiedMap[k] = v
	}
	return copiedMap
}

func (m *RWMap[K, V]) Len() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.data)
}

func LoadFromJsonString[K comparable, V any](m *RWMap[K, V], jsonStr string) error {
	return m.replaceFromJSON([]byte(jsonStr))
}

func LoadFromJsonStringWithCallback[K comparable, V any](m *RWMap[K, V], jsonStr string, onSuccess func()) error {
	if err := m.replaceFromJSON([]byte(jsonStr)); err != nil {
		return err
	}
	if onSuccess != nil {
		onSuccess()
	}
	return nil
}

// replaceFromJSON publishes a fully parsed map in one assignment. Failed
// updates leave the last known-good value available to concurrent readers.
func (m *RWMap[K, V]) replaceFromJSON(data []byte) error {
	parsed := make(map[K]V)
	if err := common.Unmarshal(data, &parsed); err != nil {
		return err
	}
	if parsed == nil {
		parsed = make(map[K]V)
	}
	m.mutex.Lock()
	m.data = parsed
	m.mutex.Unlock()
	return nil
}

func (m *RWMap[K, V]) MarshalJSONString() string {
	bytes, err := m.MarshalJSON()
	if err != nil {
		return "{}"
	}
	return string(bytes)
}
