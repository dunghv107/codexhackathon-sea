package response

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
)

func TestResponseEnvelopesAndErrorRedaction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name    string
		write   gin.HandlerFunc
		status  int
		success bool
		code    string
	}{
		{"ok", func(c *gin.Context) { OK(c, map[string]int{"count": 2}, "done") }, 200, true, ""},
		{"created", func(c *gin.Context) { Created(c, map[string]int{"id": 3}, "created") }, 201, true, ""},
		{"validation", func(c *gin.Context) { ValidationError(c, []FieldError{{Field: "amount", Message: "must be positive"}}) }, 400, false, "VALIDATION_ERROR"},
		{"client", func(c *gin.Context) { WriteErrorResponse(c, apperror.ErrForbidden) }, 403, false, "FORBIDDEN"},
		{"server", func(c *gin.Context) {
			WriteErrorResponse(c, apperror.ErrInternalServerError.WithError(errors.New("fixture-private-cause")))
		}, 500, false, "INTERNAL_ERROR"},
		{"unexpected", func(c *gin.Context) { WriteErrorResponse(c, errors.New("fixture-private-cause")) }, 500, false, "INTERNAL_ERROR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.GET("/", func(c *gin.Context) { c.Set("user_id", 7) }, tc.write)
			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/?secret=fixture-private-query", nil)
			req.Header.Set("X-Request-ID", "fixture-request")
			r.ServeHTTP(w, req)
			var body APIResponse[map[string]int]
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != tc.status || body.IsSuccess != tc.success {
				t.Fatalf("status%d body%+v", w.Code, body)
			}
			if tc.code != "" && (body.Error == nil || body.Error.Code != tc.code) {
				t.Fatalf("wrong error %+v", body.Error)
			}
			if tc.name == "ok" && body.Data["count"] != 2 {
				t.Fatal("success payload missing")
			}
			if tc.name == "created" && body.Data["id"] != 3 {
				t.Fatal("created payload missing")
			}
			if tc.name == "validation" && (len(body.Error.Fields) != 1 || body.Error.Fields[0].Field != "amount") {
				t.Fatal("validation details missing")
			}
			if strings.Contains(w.Body.String(), "fixture-private") {
				t.Fatal("private cause leaked")
			}
		})
	}
	r := gin.New()
	r.DELETE("/", NoContent)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("DELETE", "/", nil))
	if w.Code != 204 || w.Body.Len() != 0 {
		t.Fatalf("no-content contract %d %q", w.Code, w.Body.String())
	}
}
