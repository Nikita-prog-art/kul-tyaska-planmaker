package web

import (
	"context"

	"kultyaska/internal/store"
)

func withUser(ctx context.Context, u *store.User) context.Context {
	return context.WithValue(ctx, ctxUser, u)
}

func userFrom(ctx context.Context) *store.User {
	u, _ := ctx.Value(ctxUser).(*store.User)
	return u
}
