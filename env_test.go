package mercadopagopix

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := "# comment\n\nACCESS_TOKEN=TEST-abc\nexport A=1\nB=\"quoted # not comment\"\nC='x'\nD=val # inline\nEMPTY=\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"ACCESS_TOKEN": "TEST-abc", "A": "1", "B": "quoted # not comment", "C": "x", "D": "val", "EMPTY": ""}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}

	os.WriteFile(path, []byte("NOEQUALS\n"), 0o600)
	if _, err := ReadEnvFile(path); err == nil {
		t.Error("expected error for line without '='")
	}
}

func TestLoadEnvFileDoesNotOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(path, []byte("MPPIX_X=fromfile\nMPPIX_Y=fromfile\n"), 0o600)
	t.Setenv("MPPIX_X", "preset")
	t.Setenv("MPPIX_Y", "")
	os.Unsetenv("MPPIX_Y")
	if err := LoadEnvFile(path); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("MPPIX_X") != "preset" || os.Getenv("MPPIX_Y") != "fromfile" {
		t.Errorf("X=%q Y=%q", os.Getenv("MPPIX_X"), os.Getenv("MPPIX_Y"))
	}
}

func TestNewFromEnvFallbacks(t *testing.T) {
	for _, k := range envTokenVars {
		t.Setenv(k, "")
	}
	if _, err := NewFromEnv(); !errors.Is(err, ErrMissingToken) {
		t.Fatalf("err = %v, want ErrMissingToken", err)
	}
	t.Setenv("ACCESS_TOKEN", "TEST-x")
	c, err := NewFromEnv()
	if err != nil || !c.TestMode() {
		t.Fatalf("c=%v err=%v", c, err)
	}
	t.Setenv("MERCADOPAGO_ACCESS_TOKEN", "APP_USR-y")
	if c, _ := NewFromEnv(); c.TestMode() {
		t.Error("MERCADOPAGO_ACCESS_TOKEN should take precedence")
	}
}

func TestMe(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/me" {
			t.Errorf("path = %s", r.URL.Path)
		}
		io.WriteString(w, `{"id":10,"nickname":"TESTUSER1","site_id":"MLB","tags":["normal","test_user"]}`)
	})
	a, err := c.Me(context.Background())
	if err != nil || a.ID != 10 || !a.IsTestUser() {
		t.Fatalf("a=%+v err=%v", a, err)
	}
}

func TestAPIErrorHints(t *testing.T) {
	e := parseAPIError(403, []byte(`{"message":"Payer email forbidden","error":"forbidden","status":403,"cause":[{"code":4390,"description":"Payer email forbidden"}]}`))
	if !e.HasCause("4390") || e.HasCause("1") {
		t.Errorf("HasCause wrong: %+v", e.Causes)
	}
	if !strings.Contains(e.Error(), "hint:") {
		t.Errorf("missing hint: %s", e.Error())
	}
	single := parseAPIError(400, []byte(`{"message":"bad","cause":{"code":"x1","description":"single"}}`))
	if len(single.Causes) != 1 || single.Causes[0].Description != "single" {
		t.Errorf("single cause object not parsed: %+v", single.Causes)
	}
	plain := parseAPIError(502, []byte("<html>bad gateway</html>"))
	if plain.Message != "<html>bad gateway</html>" {
		t.Errorf("non-JSON body: %q", plain.Message)
	}
}
