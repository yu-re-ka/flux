package secrets

import (
	"context"

	"github.com/influxdata/flux/codes"
	"github.com/influxdata/flux/dependencies"
	"github.com/influxdata/flux/internal/errors"
	"github.com/influxdata/flux/semantic"
	"github.com/influxdata/flux/values"
)

type key int

const (
	secretKey key = iota
)

// New construct a secret object identifier from the key.
func New(key string) values.Value {
	sig := semantic.NewFunctionPolyType(semantic.FunctionPolySignature{
		Return: semantic.String,
	})
	return values.NewFunction(key, sig, func(ctx context.Context, deps dependencies.Interface, args values.Object) (values.Value, error) {
		allowed := ctx.Value(secretKey)
		if allowed == nil {
			return nil, errors.Newf(codes.PermissionDenied, "secret key %q must be used in a secret-aware context", key)
		} else if !allowed.(bool) {
			return nil, errors.Newf(codes.Invalid, "secret key %q cannot be used in a function with side-effects", key)
		}

		// todo(jsternberg): evaluate the secret using the secret service.
		return nil, errors.New(codes.Unimplemented, "implement me")
	}, false)
}

// Call will evaluate a function in a secret-aware context.
//
// If the function call has a side-effect, it will mark the function as unable
// to access the secret key and will return an appropriate error if
// a secret is used within the function.
func Call(ctx context.Context, fn values.Function, deps dependencies.Interface, args values.Object) (values.Value, error) {
	ctx = context.WithValue(ctx, secretKey, fn.HasSideEffect())
	return fn.Call(ctx, deps, args)
}
