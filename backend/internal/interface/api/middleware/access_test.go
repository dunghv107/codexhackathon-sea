package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/valueobject"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
)

type accessAuth struct {
	service.AuthService
	token string
	err   error
}

func (s *accessAuth) Authenticate(_ context.Context, token string) (*valueobject.JWTClaims, error) {
	s.token = token
	return &valueobject.JWTClaims{UserID: 7, Username: "fixture", Role: entity.UserRoleAdmin}, s.err
}

func TestAuthenticationRejectsMalformedHeadersAndPreservesVerifiedIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		header string
		err    error
		status int
		token  string
	}{
		{"", nil, 401, ""}, {"Basic token", nil, 401, ""}, {"Bearer", nil, 401, ""},
		{"Bearer token extra", nil, 401, ""}, {"Bearer valid", apperror.ErrTokenExpired, 401, "valid"},
		{"Bearer valid", nil, 200, "valid"},
	} {
		t.Run(tc.header+http.StatusText(tc.status), func(t *testing.T) {
			auth := &accessAuth{err: tc.err}
			m := New(auth, nil, nil, nil, "")
			r := gin.New()
			r.GET("/", m.Auth(), func(c *gin.Context) {
				if GetUserID(c) != 7 || GetUsername(c) != "fixture" || GetRole(c) != entity.UserRoleAdmin {
					t.Error("verified identity was not available to the handler")
				}
				c.String(200, "allowed")
			})
			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Authorization", tc.header)
			r.ServeHTTP(w, req)
			if w.Code != tc.status || auth.token != tc.token {
				t.Fatalf("status %d token %q", w.Code, auth.token)
			}
			if tc.status != 200 && strings.Contains(w.Body.String(), "allowed") {
				t.Fatal("denied handler executed")
			}
		})
	}
}

func TestAdminRolesAndMissingContext(t *testing.T) {
	for _, tc := range []struct {
		claims any
		status int
	}{
		{nil, 401}, {"wrong type", 401}, {&valueobject.JWTClaims{Role: "buyer"}, 403},
		{&valueobject.JWTClaims{Role: entity.UserRoleAdmin}, 204},
		{&valueobject.JWTClaims{Role: entity.UserRoleSystemAdmin}, 204},
	} {
		r := gin.New()
		r.GET("/", func(c *gin.Context) {
			if tc.claims != nil {
				c.Set(string(UserContextKey), tc.claims)
			}
		},
			New(nil, nil, nil, nil, "").RequireAdmin(), func(c *gin.Context) { c.Status(204) })
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != tc.status {
			t.Fatalf("%v: got %d", tc.claims, w.Code)
		}
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if GetUserID(c) != 0 || GetUsername(c) != "" || GetRole(c) != "" || GetBuyer(c) != nil || GetSeller(c) != nil || GetShop(c) != nil {
		t.Fatal("missing identity must not invent a profile")
	}
}

type accessProfiles struct {
	service.ProfileService
	status string
	err    error
	userID uint
}

func (s *accessProfiles) ResolveBuyer(_ context.Context, userID uint) (*entity.Buyer, error) {
	s.userID = userID
	return &entity.Buyer{BuyerID: "buyer", Status: s.status}, s.err
}
func (s *accessProfiles) ResolveSeller(_ context.Context, userID uint) (*entity.Seller, error) {
	s.userID = userID
	return &entity.Seller{SellerID: "seller", Status: s.status}, s.err
}

func TestProfileStatusAllowsReadsButSuspensionBlocksWrites(t *testing.T) {
	for _, seller := range []bool{false, true} {
		for _, tc := range []struct {
			method, status string
			err            error
			want           int
		}{
			{"GET", entity.AccountStatusSuspended, nil, 204}, {"HEAD", entity.AccountStatusSuspended, nil, 204},
			{"OPTIONS", entity.AccountStatusSuspended, nil, 204}, {"POST", entity.AccountStatusSuspended, nil, 403},
			{"PATCH", entity.AccountStatusActive, nil, 204}, {"GET", entity.AccountStatusActive, apperror.ErrForbidden, 403},
		} {
			profiles := &accessProfiles{status: tc.status, err: tc.err}
			m := New(nil, profiles, nil, nil, "")
			guard := m.RequireBuyer()
			if seller {
				guard = m.RequireSeller()
			}
			r := gin.New()
			r.Handle(tc.method, "/", func(c *gin.Context) { c.Set(string(UserContextKey), &valueobject.JWTClaims{UserID: 9}) }, guard, func(c *gin.Context) {
				if seller {
					if GetSeller(c).SellerID != "seller" {
						t.Error("wrong seller")
					}
				} else if GetBuyer(c).BuyerID != "buyer" {
					t.Error("wrong buyer")
				}
				c.Status(204)
			})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(tc.method, "/", nil))
			if w.Code != tc.want || profiles.userID != 9 {
				t.Fatalf("seller=%v method=%s got status%d user%d", seller, tc.method, w.Code, profiles.userID)
			}
		}
	}
}

type accessSeller struct {
	service.SellerService
	err              error
	sellerID, shopID string
}

func (s *accessSeller) GetOwnedShop(_ context.Context, sellerID, shopID string) (*entity.Shop, error) {
	s.sellerID, s.shopID = sellerID, shopID
	return &entity.Shop{ShopID: shopID}, s.err
}

func TestOwnedShopCannotUseClientSuppliedSellerIdentity(t *testing.T) {
	for _, denied := range []bool{false, true} {
		s := &accessSeller{}
		if denied {
			s.err = apperror.ErrNotFound
		}
		r := gin.New()
		r.GET("/shops/:shopId", func(c *gin.Context) { c.Set(string(SellerContextKey), &entity.Seller{SellerID: "verified-seller"}) }, New(nil, nil, s, nil, "").RequireOwnedShop(), func(c *gin.Context) {
			if GetShop(c).ShopID != "target" {
				t.Error("wrong shop context")
			}
			c.Status(204)
		})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/shops/target?sellerID=attacker", nil))
		want := 204
		if denied {
			want = 404
		}
		if w.Code != want || s.sellerID != "verified-seller" || s.shopID != "target" {
			t.Fatalf("status%d lookup%+v", w.Code, s)
		}
	}
}

func TestCORSOriginAndPreflightContracts(t *testing.T) {
	for _, tc := range []struct {
		allowed, origin, method, wantOrigin string
		status                              int
	}{
		{"*", "", "GET", "*", 200}, {"*", "https://browser.example", "GET", "https://browser.example", 200},
		{" https://allowed.example, ,https://second.example ", "https://second.example", "OPTIONS", "https://second.example", 204},
		{"https://allowed.example", "https://attacker.example", "GET", "", 200},
		{"https://allowed.example", "", "GET", "", 200},
	} {
		r := gin.New()
		r.Use(New(nil, nil, nil, nil, tc.allowed).CORS())
		r.Any("/", func(c *gin.Context) { c.String(200, "handler") })
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, "/", nil)
		req.Header.Set("Origin", tc.origin)
		r.ServeHTTP(w, req)
		if w.Code != tc.status || w.Header().Get("Access-Control-Allow-Origin") != tc.wantOrigin {
			t.Fatalf("origin=%q status%d headers%v", tc.origin, w.Code, w.Header())
		}
		if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("X-Frame-Options") != "DENY" {
			t.Fatal("security headers missing")
		}
		if tc.wantOrigin == "*" && w.Header().Get("Access-Control-Allow-Credentials") != "false" {
			t.Fatal("wildcard origin must not allow credentials")
		}
		if tc.method == "OPTIONS" && w.Body.Len() != 0 {
			t.Fatal("preflight reached handler")
		}
	}
}

func TestRecoveryDoesNotExposePanicToClient(t *testing.T) {
	r := gin.New()
	r.Use(Recovery())
	r.GET("/", func(*gin.Context) { panic("fixture-private-panic") })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 500 || !strings.Contains(w.Body.String(), "INTERNAL_ERROR") || strings.Contains(w.Body.String(), "fixture-private-panic") {
		t.Fatalf("unexpected recovery response %d %s", w.Code, w.Body.String())
	}
}
