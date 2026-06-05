package admin

import "context"

type adminContextKey string

const currentAdminKey adminContextKey = "current_admin"

type CurrentAdmin struct {
	AdminID  string
	Email    string
	Nickname string
}

func WithCurrentAdmin(ctx context.Context, admin CurrentAdmin) context.Context {
	return context.WithValue(ctx, currentAdminKey, admin)
}

func CurrentAdminFromContext(ctx context.Context) (CurrentAdmin, bool) {
	value := ctx.Value(currentAdminKey)
	if value == nil {
		return CurrentAdmin{}, false
	}
	admin, ok := value.(CurrentAdmin)
	if !ok || admin.AdminID == "" {
		return CurrentAdmin{}, false
	}
	return admin, true
}
