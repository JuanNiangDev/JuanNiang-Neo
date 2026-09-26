package middleware

import (
	"context"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
)

func TestRequireAdmin(t *testing.T) {
	for _, tc := range []struct {
		name string
		role string
		forbidden bool
	}{
		{name: "missing role", forbidden: true},
		{name: "ordinary user", role: "user", forbidden: true},
		{name: "mixed case", role: "Admin", forbidden: true},
		{name: "administrator", role: "admin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := app.NewContext(0)
			if tc.role != "" {
				c.Set("role", tc.role)
			}
			RequireAdmin()(context.Background(), c)
			gotForbidden := c.Response.StatusCode() == http.StatusForbidden
			if gotForbidden != tc.forbidden {
				t.Fatalf("role %q: forbidden = %v, want %v", tc.role, gotForbidden, tc.forbidden)
			}
		})
	}
}
