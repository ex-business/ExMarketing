package plugin

import (
	"context"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

type ActivityPlugin interface {
	Code() string
	BuildContext(context.Context, marketing.ActivityEvent) (map[string]any, error)
	BeforeEvaluate(context.Context, map[string]any) error
	AfterQualified(context.Context, map[string]any) error
}

type Registry struct {
	plugins map[string]ActivityPlugin
}

func NewRegistry() *Registry {
	return &Registry{plugins: map[string]ActivityPlugin{}}
}

func (r *Registry) Register(plugin ActivityPlugin) {
	r.plugins[plugin.Code()] = plugin
}

func (r *Registry) Codes() []string {
	out := make([]string, 0, len(r.plugins))
	for code := range r.plugins {
		out = append(out, code)
	}
	return out
}
