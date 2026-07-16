package graph

import (
	"encoding/json"
	"fmt"
	"sync"
)

// State 工作流状态接口
type State interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{})
	GetString(key string) string
	GetInt(key string) int
	GetFloat(key string) float64
	GetBool(key string) bool
	GetStringSlice(key string) []string
	Clone() State
	Merge(other State) State
	ToMap() map[string]interface{}
	MarshalJSON() ([]byte, error)
}

// MemoryState 内存状态实现
type MemoryState struct {
	mu   sync.RWMutex
	data map[string]interface{}
}

// NewMemoryState 创建新的内存状态
func NewMemoryState() *MemoryState {
	return &MemoryState{
		data: make(map[string]interface{}),
	}
}

// NewMemoryStateFromMap 从 map 创建状态
func NewMemoryStateFromMap(data map[string]interface{}) *MemoryState {
	state := &MemoryState{
		data: make(map[string]interface{}),
	}
	for k, v := range data {
		state.data[k] = v
	}
	return state
}

func (s *MemoryState) Get(key string) (interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, exists := s.data[key]
	return val, exists
}

func (s *MemoryState) Set(key string, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}

func (s *MemoryState) GetString(key string) string {
	val, exists := s.Get(key)
	if !exists {
		return ""
	}
	if str, ok := val.(string); ok {
		return str
	}
	return fmt.Sprintf("%v", val)
}

func (s *MemoryState) GetInt(key string) int {
	val, exists := s.Get(key)
	if !exists {
		return 0
	}
	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	default:
		return 0
	}
}

func (s *MemoryState) GetFloat(key string) float64 {
	val, exists := s.Get(key)
	if !exists {
		return 0
	}
	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	default:
		return 0
	}
}

func (s *MemoryState) GetBool(key string) bool {
	val, exists := s.Get(key)
	if !exists {
		return false
	}
	if b, ok := val.(bool); ok {
		return b
	}
	return false
}

func (s *MemoryState) GetStringSlice(key string) []string {
	val, exists := s.Get(key)
	if !exists {
		return nil
	}
	if slice, ok := val.([]string); ok {
		return slice
	}
	return nil
}

func (s *MemoryState) Clone() State {
	s.mu.RLock()
	defer s.mu.RUnlock()

	newState := &MemoryState{
		data: make(map[string]interface{}),
	}
	for k, v := range s.data {
		newState.data[k] = v
	}
	return newState
}

func (s *MemoryState) Merge(other State) State {
	result := s.Clone().(*MemoryState)

	otherMap := other.ToMap()
	for k, v := range otherMap {
		result.Set(k, v)
	}

	return result
}

func (s *MemoryState) ToMap() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]interface{})
	for k, v := range s.data {
		result[k] = v
	}
	return result
}

// MarshalJSON 实现 JSON 序列化（用于检查点持久化）
func (s *MemoryState) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.ToMap())
}

// UnmarshalMemoryState 从 JSON 反序列化
func UnmarshalMemoryState(data []byte) (*MemoryState, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return NewMemoryStateFromMap(m), nil
}

// JSONState JSON 序列化状态（用于检查点存储）
type JSONState struct {
	Data map[string]interface{} `json:"data"`
}

// ToJSON 将状态转换为 JSON
func (s *MemoryState) ToJSON() ([]byte, error) {
	return json.Marshal(s.ToMap())
}

// FromJSON 从 JSON 恢复状态
func FromJSON(data []byte) (State, error) {
	return UnmarshalMemoryState(data)
}
