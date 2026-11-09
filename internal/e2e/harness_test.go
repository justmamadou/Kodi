// Package e2e teste l'API complète sur un vrai PostgreSQL, connecté avec le rôle applicatif cf_app :
// la RLS, les triggers et les contraintes s'appliquent donc exactement comme en production.
package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justmamadou/Kodi/internal/app"
	"github.com/justmamadou/Kodi/internal/httpx"
	"github.com/justmamadou/Kodi/internal/platform/config"
	"github.com/justmamadou/Kodi/internal/platform/db"
	"github.com/justmamadou/Kodi/internal/platform/migrate"
	"github.com/justmamadou/Kodi/internal/platform/storage"
)

// Rôle propriétaire ; une base dédiée est créée pour chaque exécution.
const defaultAdminURL = "postgres://cf_owner:cf_owner@localhost:5432/kodi?sslmode=disable"

var (
	env struct {
		server   *httptest.Server
		store    *fakeStorage
		mail     *fakeMailer
		appPool  *pgxpool.Pool
		ownerURL string
		ownerDB  *pgxpool.Pool
	}
	counter   int
	counterMu sync.Mutex
)

func TestMain(m *testing.M) {
	code, err := setup(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "préparation des tests en échec :", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func setup(m *testing.M) (int, error) {
	// Sorties de test lisibles ; E2E_LOG=1 garde les journaux de l'API pour diagnostiquer une erreur 500.
	if os.Getenv("E2E_LOG") == "" {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	}
	ctx := context.Background()
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		adminURL = defaultAdminURL
	}
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		return 0, err
	}
	defer admin.Close()
	if err := admin.Ping(ctx); err != nil {
		return 0, fmt.Errorf("PostgreSQL injoignable (docker compose up -d postgres ?) : %w", err)
	}

	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return 0, err
	}
	dbName := "cf_test_" + hex.EncodeToString(suffix)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
		return 0, err
	}
	defer func() {
		_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName+" WITH (FORCE)")
	}()
	// Le rôle applicatif est global au cluster : on s'assure qu'il peut se connecter.
	if _, err := admin.Exec(ctx, `DO $$ BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'cf_app') THEN
			CREATE ROLE cf_app LOGIN PASSWORD 'cf_app' NOSUPERUSER NOBYPASSRLS;
		ELSE
			ALTER ROLE cf_app LOGIN PASSWORD 'cf_app';
		END IF;
	END $$`); err != nil {
		return 0, err
	}

	u, err := url.Parse(adminURL)
	if err != nil {
		return 0, err
	}
	u.Path = "/" + dbName
	env.ownerURL = u.String()
	if err := migrate.Up(env.ownerURL); err != nil {
		return 0, fmt.Errorf("migrations : %w", err)
	}
	env.ownerDB, err = pgxpool.New(ctx, env.ownerURL)
	if err != nil {
		return 0, err
	}
	defer env.ownerDB.Close()

	appURL := *u
	appURL.User = url.UserPassword("cf_app", "cf_app")
	env.appPool, err = db.Connect(ctx, appURL.String())
	if err != nil {
		return 0, err
	}
	defer env.appPool.Close()

	env.store = newFakeStorage()
	env.mail = &fakeMailer{}
	router, err := app.NewRouter(app.Deps{
		Config: config.Config{
			Env: "test", AppBaseURL: "http://app.test", AuthRatePerMinute: 100000,
		},
		Tx: db.NewTxManager(env.appPool), Storage: env.store, Mailer: env.mail,
	})
	if err != nil {
		return 0, err
	}
	env.server = httptest.NewServer(router)
	defer env.server.Close()
	return m.Run(), nil
}

// fakeStorage remplace l'object storage : le stockage S3 réel n'est pas nécessaire pour tester
// le contrôle d'accès et la vérification des octets déposés.
type fakeStorage struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newFakeStorage() *fakeStorage { return &fakeStorage{objects: map[string][]byte{}} }

func (f *fakeStorage) put(key string, body []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = body
}

func (f *fakeStorage) PresignPut(_ context.Context, key, _ string, _ time.Duration) (string, error) {
	return "https://s3.test/put/" + key, nil
}

func (f *fakeStorage) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://s3.test/get/" + key, nil
}

func (f *fakeStorage) Stat(_ context.Context, key string) (storage.ObjectInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.objects[key]
	if !ok {
		return storage.ObjectInfo{}, storage.ErrNotFound
	}
	return storage.ObjectInfo{Size: int64(len(b))}, nil
}

func (f *fakeStorage) ReadHead(_ context.Context, key string, n int64) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b := f.objects[key]
	if int64(len(b)) > n {
		b = b[:n]
	}
	return b, nil
}

func unique() string {
	counterMu.Lock()
	defer counterMu.Unlock()
	counter++
	suffix := make([]byte, 3)
	_, _ = rand.Read(suffix)
	return fmt.Sprintf("%d%s", counter, hex.EncodeToString(suffix))
}

// client est un navigateur de test : il conserve ses cookies et renvoie le jeton CSRF.
type client struct {
	t    *testing.T
	http *http.Client
}

func newClient(t *testing.T) *client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &client{t: t, http: &http.Client{Jar: jar}}
}

type response struct {
	status int
	body   []byte
	header http.Header
}

func (r response) json() map[string]any {
	var out map[string]any
	_ = json.Unmarshal(r.body, &out)
	return out
}

func (r response) code() string {
	s, _ := r.json()["code"].(string)
	return s
}

func (r response) str(key string) string {
	s, _ := r.json()[key].(string)
	return s
}

func (r response) num(key string) int64 {
	n, _ := r.json()[key].(float64)
	return int64(n)
}

func (r response) items() []map[string]any {
	raw, _ := r.json()["items"].([]any)
	out := make([]map[string]any, len(raw))
	for i, item := range raw {
		out[i], _ = item.(map[string]any)
	}
	return out
}

func (c *client) do(method, path string, body any, headers ...string) response {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, env.server.URL+path, reader)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	u, _ := url.Parse(env.server.URL)
	for _, cookie := range c.http.Jar.Cookies(u) {
		if cookie.Name == httpx.CSRFCookie {
			req.Header.Set(httpx.CSRFHeader, cookie.Value)
		}
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatal(err)
	}
	return response{status: resp.StatusCode, body: data, header: resp.Header}
}

// must exécute une requête et échoue si le statut n'est pas celui attendu.
func (c *client) must(want int, method, path string, body any, headers ...string) response {
	c.t.Helper()
	r := c.do(method, path, body, headers...)
	if r.status != want {
		c.t.Fatalf("%s %s : statut %d, attendu %d\n%s", method, path, r.status, want, r.body)
	}
	return r
}

func expectCode(t *testing.T, r response, status int, code string) {
	t.Helper()
	if r.status != status || (code != "" && r.code() != code) {
		t.Fatalf("attendu %d/%q, obtenu %d/%q\n%s", status, code, r.status, r.code(), r.body)
	}
}

// tenant est une entreprise de test avec un membre par rôle.
type tenant struct {
	name       string
	owner      *client
	ownerID    string
	manager    *client
	managerID  string
	accountant *client
	accountID  string
	worker     *client
	workerID   string
	viewer     *client
	viewerID   string
	categoryID string
}

const testPassword = "motdepasse-solide-42"

func newTenant(t *testing.T) *tenant {
	t.Helper()
	id := unique()
	tn := &tenant{name: "Entreprise " + id, owner: newClient(t)}
	r := tn.owner.must(201, "POST", "/api/v1/auth/signup", map[string]any{
		"organization_name": tn.name, "full_name": "Moussa Diop",
		"email": "owner-" + id + "@example.test", "password": testPassword,
	})
	user, _ := r.json()["user"].(map[string]any)
	tn.ownerID, _ = user["id"].(string)
	tn.manager, tn.managerID = tn.invite(t, "manager")
	tn.accountant, tn.accountID = tn.invite(t, "accountant")
	tn.worker, tn.workerID = tn.invite(t, "site_worker")
	tn.viewer, tn.viewerID = tn.invite(t, "viewer")
	cats := tn.owner.must(200, "GET", "/api/v1/categories", nil).items()
	if len(cats) != 6 {
		t.Fatalf("6 catégories par défaut attendues, %d trouvées", len(cats))
	}
	tn.categoryID, _ = cats[0]["id"].(string)
	return tn
}

func (tn *tenant) invite(t *testing.T, role string) (*client, string) {
	t.Helper()
	id := unique()
	r := tn.owner.must(201, "POST", "/api/v1/invitations", map[string]any{
		"full_name": "Membre " + id, "email": role + "-" + id + "@example.test", "role": role,
	})
	c := newClient(t)
	token := tokenFrom(t, r.str("invite_url"))
	c.must(200, "POST", "/api/v1/auth/invitations/"+token+"/accept", map[string]any{"password": testPassword})
	return c, r.str("user_id")
}

func tokenFrom(t *testing.T, inviteURL string) string {
	t.Helper()
	i := strings.LastIndex(inviteURL, "/")
	if i < 0 {
		t.Fatalf("lien d'invitation invalide : %q", inviteURL)
	}
	return inviteURL[i+1:]
}

// newProject crée un chantier géré par le manager du tenant, avec le chef de chantier et le lecteur affectés.
func (tn *tenant) newProject(t *testing.T) string {
	t.Helper()
	id := unique()
	r := tn.owner.must(201, "POST", "/api/v1/projects", map[string]any{
		"reference": "CH-" + id, "name": "Villa " + id, "client_name": "M. Sarr", "address": "Almadies, Dakar",
		"start_date": "2026-01-15", "status": "in_progress", "manager_id": tn.managerID,
	})
	projectID := r.str("id")
	for _, member := range []string{tn.workerID, tn.viewerID} {
		tn.owner.must(200, "PUT", "/api/v1/projects/"+projectID+"/members/"+member, map[string]any{})
	}
	return projectID
}

// publishBudget crée et publie un budget d'un seul poste : quantity × unitPrice.
func (tn *tenant) publishBudget(t *testing.T, projectID string, quantity string, unitPrice int64) string {
	t.Helper()
	v := tn.manager.must(201, "POST", "/api/v1/projects/"+projectID+"/budget/versions", nil)
	versionID := v.str("id")
	tn.manager.must(201, "POST", "/api/v1/budget-versions/"+versionID+"/lines", map[string]any{
		"category_id": tn.categoryID, "label": "Ciment CPJ 45", "quantity": quantity, "unit": "sac", "unit_price": unitPrice,
	})
	tn.manager.must(200, "POST", "/api/v1/budget-versions/"+versionID+"/publish", nil)
	return versionID
}

// newExpense crée un brouillon de dépense.
func (tn *tenant) newExpense(t *testing.T, c *client, projectID string, amount int64) string {
	t.Helper()
	r := c.must(201, "POST", "/api/v1/projects/"+projectID+"/expenses", map[string]any{
		"category_id": tn.categoryID, "description": "Ciment 50 sacs", "amount": amount, "expense_date": "2026-11-03",
	})
	return r.str("id")
}

// approvedExpense crée, soumet et fait approuver une dépense.
func (tn *tenant) approvedExpense(t *testing.T, projectID string, amount int64) string {
	t.Helper()
	id := tn.newExpense(t, tn.worker, projectID, amount)
	tn.worker.must(200, "POST", "/api/v1/expenses/"+id+"/submit", nil)
	tn.accountant.must(200, "POST", "/api/v1/expenses/"+id+"/approve", nil)
	return id
}

// allowed lance fn en concurrence et retourne le nombre de réponses 2xx.
func concurrently(n int, fn func(i int) response) (ok int, statuses []int) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := fn(i)
			mu.Lock()
			defer mu.Unlock()
			statuses = append(statuses, r.status)
			if r.status >= 200 && r.status < 300 {
				ok++
			}
		}()
	}
	wg.Wait()
	return ok, statuses
}
