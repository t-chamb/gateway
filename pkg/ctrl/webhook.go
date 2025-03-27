package ctrl

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var ErrUnexpectedType = errors.New("unexpected type")

type TypedDefaulter[T runtime.Object] interface {
	Default(ctx context.Context, obj T) error
}

func FromTypedDefaulter[T runtime.Object](typed TypedDefaulter[T]) admission.CustomDefaulter {
	return &untypedDefaulter[T]{
		defaulter: typed,
	}
}

type untypedDefaulter[T runtime.Object] struct {
	defaulter TypedDefaulter[T]
}

var _ admission.CustomDefaulter = (*untypedDefaulter[runtime.Object])(nil)

func (t *untypedDefaulter[T]) Default(ctx context.Context, obj runtime.Object) error {
	typed, ok := obj.(T)
	if !ok {
		return fmt.Errorf("%w: expected %T but got %T", ErrUnexpectedType, typed, obj)
	}

	return t.defaulter.Default(ctx, typed)
}

type TypedValidator[T runtime.Object] interface {
	ValidateCreate(ctx context.Context, obj T) (warnings admission.Warnings, err error)
	ValidateUpdate(ctx context.Context, oldObj, newObj T) (warnings admission.Warnings, err error)
	ValidateDelete(ctx context.Context, obj T) (warnings admission.Warnings, err error)
}

func FromTypedValidator[T runtime.Object](typed TypedValidator[T]) admission.CustomValidator {
	return &untypedValidator[T]{
		validator: typed,
	}
}

type untypedValidator[T runtime.Object] struct {
	validator TypedValidator[T]
}

var _ admission.CustomValidator = (*untypedValidator[runtime.Object])(nil)

func (t *untypedValidator[T]) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	typed, ok := obj.(T)
	if !ok {
		return nil, fmt.Errorf("%w: expected %T but got %T", ErrUnexpectedType, typed, obj)
	}

	return t.validator.ValidateCreate(ctx, typed)
}

func (t *untypedValidator[T]) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldTyped, ok := oldObj.(T)
	if !ok {
		return nil, fmt.Errorf("%w: old expected %T but got %T", ErrUnexpectedType, oldTyped, oldObj)
	}

	newTyped, ok := newObj.(T)
	if !ok {
		return nil, fmt.Errorf("%w: new expected %T but got %T", ErrUnexpectedType, newTyped, newObj)
	}

	return t.validator.ValidateUpdate(ctx, oldTyped, newTyped)
}

func (t *untypedValidator[T]) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	typed, ok := obj.(T)
	if !ok {
		return nil, fmt.Errorf("%w: expected %T but got %T", ErrUnexpectedType, typed, obj)
	}

	return t.validator.ValidateDelete(ctx, typed)
}
