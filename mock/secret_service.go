package mock

import (
	"context"
	"github.com/influxdata/flux/codes"
	"github.com/influxdata/flux/dependencies"
	"github.com/influxdata/flux/internal/errors"
)

type SecretService map[string]string

func (s SecretService) LoadSecret(ctx context.Context, k string) (string, error) {
	v, ok := s[k]
	if ok {
		return v, nil
	}
	return "", errors.Newf(codes.NotFound, "secret key %q not found", k)
}

func (s SecretService) Inject(ctx context.Context) context.Context {
	return dependencies.WithSecretService(ctx, s)
}
