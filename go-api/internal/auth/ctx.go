package auth

import "context"

type userKey struct{}
type roleKey struct{}

func WithPrincipal(ctx context.Context, username, role string) context.Context {
	return context.WithValue(context.WithValue(ctx, userKey{}, username), roleKey{}, role)
}

func Username(ctx context.Context) string {
	if u, _ := ctx.Value(userKey{}).(string); u != "" {
		return u
	}
	return "unknown"
}

func Role(ctx context.Context) string { r, _ := ctx.Value(roleKey{}).(string); return r }
