package apperror

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestErrorVariantsDoNotMutateSharedSentinelOrExposeCauseInJSON(t *testing.T) {
	cause := errors.New("fixture-private-cause")
	wrapped := ErrBadRequest.WithError(cause)
	custom := wrapped.WithMessage("safe message")
	if ErrBadRequest.Err != nil || custom.Message != "safe message" || custom.Err != cause || custom.Code != ErrBadRequest.Code || custom.HTTPStatus != 400 {
		t.Fatalf("error contract changed %+v", custom)
	}
	if !strings.Contains(custom.Error(), cause.Error()) || strings.Contains(ErrBadRequest.Error(), cause.Error()) {
		t.Fatal("debug error context missing or sentinel mutated")
	}
	if wrapped.Message != ErrBadRequest.Message {
		t.Fatal("WithMessage mutated its receiver")
	}
	data, err := json.Marshal(custom)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), cause.Error()) || strings.Contains(string(data), "HTTPStatus") {
		t.Fatal("internal error fields leaked into JSON")
	}
}
