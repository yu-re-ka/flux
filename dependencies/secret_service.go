package dependencies

import (
	"context"
)

type key int

const (
	secretServiceKey key = iota
)

// SecretService generalizes the process of looking up secrets based on a key.
type SecretService interface {
	// LoadSecret retrieves the secret value v found at key k given the calling context ctx.
	LoadSecret(ctx context.Context, k string) (string, error)
}

// WithSecretService will inject a SecretService into a context.
func WithSecretService(ctx context.Context, ss SecretService) context.Context {
	return context.WithValue(ctx, secretServiceKey, ss)
}

// SecretServiceFromContext will retrieve a SecretService from the
// context. It will return nil if none has been set.
func SecretServiceFromContext(ctx context.Context) SecretService {
	ss, _ := ctx.Value(secretServiceKey).(SecretService)
	return ss
}
