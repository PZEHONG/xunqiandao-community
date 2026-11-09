package web

import (
	"database/sql"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"xixunyunsign/autosign"
	"xixunyunsign/platformauth"
)

func TestCommunityHasNoPrivateRoutesOrSchema(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "community.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	auth, err := platformauth.New(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := autosign.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	router := SetupRouter(&Handlers{PlatformAuth: auth, AutoSign: tasks})
	for _, route := range router.Routes() {
		for _, private := range []string{"admin", "membership", "recharge", "/api/pay", "invitation", "announcement"} {
			if strings.Contains(route.Path, private) {
				t.Fatalf("private route registered: %s", route.Path)
			}
		}
	}
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		for _, private := range []string{"admin", "membership", "invitation", "recharge", "payment", "reward"} {
			if strings.Contains(name, private) {
				t.Fatalf("private table created: %s", name)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/admin", "/admin/"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != 404 {
			t.Fatalf("private page %s returned %d", path, response.Code)
		}
	}
}
