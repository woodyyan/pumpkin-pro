package auth

import "context"

type userContextKey string

const currentUserKey userContextKey = "current_user"

type CurrentUser struct {
	UserID string
	Email  string
}

func WithCurrentUser(ctx context.Context, user CurrentUser) context.Context {
	return context.WithValue(ctx, currentUserKey, user)
}

func CurrentUserFromContext(ctx context.Context) (CurrentUser, bool) {
	value := ctx.Value(currentUserKey)
	if value == nil {
		return CurrentUser{}, false
	}
	user, ok := value.(CurrentUser)
	if !ok || user.UserID == "" {
		return CurrentUser{}, false
	}
	return user, true
}
