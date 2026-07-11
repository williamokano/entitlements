package apperr

import (
	"errors"
	"testing"
)

func TestCoerceWrapsNonAppError(t *testing.T) {
	got := Coerce(errors.New("boom"))
	if got.Kind != KindInternal {
		t.Fatalf("Kind = %v, want KindInternal", got.Kind)
	}
}

func TestCoercePreservesAppError(t *testing.T) {
	orig := NotFound("no tenant")
	if got := Coerce(orig); got != orig {
		t.Fatalf("Coerce returned a different *Error")
	}
}

func TestCoerceNilReturnsNil(t *testing.T) {
	if Coerce(nil) != nil {
		t.Fatal("Coerce(nil) != nil")
	}
}

func TestErrorsAsThroughWrap(t *testing.T) {
	sentinel := errors.New("db down")
	err := Wrap(KindConflict, sentinel, "slug taken")

	if !errors.Is(err, sentinel) {
		t.Fatal("errors.Is did not find wrapped cause")
	}
	if KindOf(err) != KindConflict {
		t.Fatalf("KindOf = %v, want KindConflict", KindOf(err))
	}
}

func TestKindString(t *testing.T) {
	cases := map[Kind]string{
		KindInternal:     "internal",
		KindValidation:   "validation",
		KindUnauthorized: "unauthorized",
		KindForbidden:    "forbidden",
		KindNotFound:     "not_found",
		KindConflict:     "conflict",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("Kind(%d).String() = %q, want %q", k, got, want)
		}
	}
}
