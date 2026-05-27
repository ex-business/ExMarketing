package marketing

import "context"

type auditSourceContextKey struct{}

func WithAuditSource(ctx context.Context, source map[string]any) context.Context {
	if len(source) == 0 {
		return ctx
	}
	copied := make(map[string]any, len(source))
	for key, value := range source {
		copied[key] = value
	}
	return context.WithValue(ctx, auditSourceContextKey{}, copied)
}

func AuditSourceFromContext(ctx context.Context) map[string]any {
	if ctx == nil {
		return nil
	}
	source, ok := ctx.Value(auditSourceContextKey{}).(map[string]any)
	if !ok || len(source) == 0 {
		return nil
	}
	copied := make(map[string]any, len(source))
	for key, value := range source {
		copied[key] = value
	}
	return copied
}
