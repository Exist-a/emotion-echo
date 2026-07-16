// Package tools 提供工作流工具接口和内置实现
package tools

import "context"

// Tool 工具接口
type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, input string) (string, error)
}

// Registry 工具注册表
type Registry struct {
	tools map[string]Tool
}

// NewRegistry 创建工具注册表
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register 注册工具
func (r *Registry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

// Get 获取工具
func (r *Registry) Get(name string) (Tool, bool) {
	tool, exists := r.tools[name]
	return tool, exists
}

// List 列出所有工具
func (r *Registry) List() []Tool {
	list := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		list = append(list, tool)
	}
	return list
}

// ToMap 转换为 graph.ToolExecutor 映射
func (r *Registry) ToMap() map[string]func(ctx context.Context, input string) (string, error) {
	m := make(map[string]func(ctx context.Context, input string) (string, error))
	for name, tool := range r.tools {
		t := tool // 闭包捕获
		m[name] = func(ctx context.Context, input string) (string, error) {
			return t.Execute(ctx, input)
		}
	}
	return m
}
