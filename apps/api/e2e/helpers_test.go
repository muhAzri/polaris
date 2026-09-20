package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"polaris-api/internal/app"
	"polaris-api/internal/database"
)

// memStorage keeps uploaded files in memory so the tests need no object
// store.
type memStorage struct{ files map[string][]byte }

func (m *memStorage) Put(_ context.Context, key string, body io.Reader, _ int64, _ string) error {
	b, err := io.ReadAll(body)
	m.files[key] = b
	return err
}

func (m *memStorage) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "http://files.test/" + key, nil
}

func (m *memStorage) Delete(_ context.Context, key string) error {
	delete(m.files, key)
	return nil
}

type env struct {
	t     *testing.T
	pool  *pgxpool.Pool
	srv   *httptest.Server
	store *memStorage
}

// newEnv connects to E2E_DATABASE_URL, wipes it, applies every migration
// and serves the real API on a local test server. The test is skipped when
// the variable is unset, and refuses to run against a database whose name
// does not look disposable, because it drops the whole public schema.
func newEnv(t *testing.T) *env {
	t.Helper()
	dsn := os.Getenv("E2E_DATABASE_URL")
	if dsn == "" {
		t.Skip("set E2E_DATABASE_URL to a throwaway database to run the end-to-end tests")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("bad E2E_DATABASE_URL: %v", err)
	}
	name := strings.ToLower(strings.TrimPrefix(u.Path, "/"))
	if !strings.Contains(name, "e2e") && !strings.Contains(name, "test") {
		t.Fatalf("refusing to wipe database %q: its name must contain \"e2e\" or \"test\"", name)
	}

	ctx := context.Background()
	pool, err := database.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	store := &memStorage{files: map[string][]byte{}}
	srv := httptest.NewServer(app.New(pool, app.Options{JWTSecret: "e2e-secret", JWTTTLHours: 1}, store))
	t.Cleanup(srv.Close)
	return &env{t: t, pool: pool, srv: srv, store: store}
}

// J is a decoded JSON object.
type J = map[string]any

func (e *env) do(method, path, token string, body any) (int, any) {
	e.t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatal(err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.srv.URL+"/api/v1"+path, reader)
	if err != nil {
		e.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return e.send(req)
}

func (e *env) send(req *http.Request) (int, any) {
	e.t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/") {
		return resp.StatusCode, string(raw)
	}
	var v any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &v)
	}
	return resp.StatusCode, v
}

func (e *env) upload(path, token, title, filename, content string) (int, any) {
	e.t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("title", title)
	part, _ := w.CreateFormFile("file", filename)
	_, _ = part.Write([]byte(content))
	_ = w.Close()
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1"+path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	return e.send(req)
}

// status runs a request and returns only its status code.
func (e *env) status(method, path, token string, body any) int {
	e.t.Helper()
	s, _ := e.do(method, path, token, body)
	return s
}

// must runs a request that has to succeed with the given status and
// returns the decoded body.
func (e *env) must(want int, method, path, token string, body any) any {
	e.t.Helper()
	s, v := e.do(method, path, token, body)
	if s != want {
		e.t.Fatalf("%s %s: status %d, want %d (%v)", method, path, s, want, v)
	}
	return v
}

func (e *env) sql(q string, args ...any) {
	e.t.Helper()
	if _, err := e.pool.Exec(context.Background(), q, args...); err != nil {
		e.t.Fatalf("sql %q: %v", q, err)
	}
}

type person struct{ token, id, email string }

// newUser registers and logs in a user, optionally promoting the site-level
// role flag (the API has no endpoint for that on purpose).
func (e *env) newUser(name, email, role string) person {
	e.t.Helper()
	e.must(201, "POST", "/auth/register", "", J{"name": name, "email": email, "password": "rahasia1234"})
	if role != "" {
		e.sql(`UPDATE users SET role = $1 WHERE email = $2`, role, email)
	}
	v := e.must(200, "POST", "/auth/login", "", J{"email": email, "password": "rahasia1234"})
	return person{token: str(v, "token"), id: str(v, "user", "id"), email: email}
}

// --- decoding helpers ---

func get(v any, keys ...string) any {
	for _, k := range keys {
		m, ok := v.(J)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}

func str(v any, keys ...string) string {
	s, _ := get(v, keys...).(string)
	return s
}

func list(v any) []any {
	l, _ := v.([]any)
	return l
}

// col extracts one field from every object in a JSON array.
func col(v any, key string) []any {
	out := []any{}
	for _, item := range list(v) {
		out = append(out, get(item, key))
	}
	return out
}

func colStr(v any, key string) []string {
	out := []string{}
	for _, x := range col(v, key) {
		s, _ := x.(string)
		out = append(out, s)
	}
	return out
}

// find returns the first object in an array whose key equals want.
func find(v any, key string, want any) J {
	for _, item := range list(v) {
		if reflect.DeepEqual(get(item, key), want) {
			m, _ := item.(J)
			return m
		}
	}
	return nil
}

// norm makes expected and decoded values comparable: JSON numbers decode
// as float64, so Go ints and string slices are converted, recursively.
func norm(v any) any {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case []string:
		out := make([]any, len(n))
		for i, s := range n {
			out[i] = s
		}
		return out
	case []any:
		out := make([]any, len(n))
		for i, x := range n {
			out[i] = norm(x)
		}
		return out
	}
	return v
}

func check(t *testing.T, name string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(norm(got), norm(want)) {
		t.Errorf("%s: got %#v, want %#v", name, got, want)
	}
}
