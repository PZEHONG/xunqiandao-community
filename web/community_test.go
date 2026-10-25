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
		if strings.Contains(route.Path, "admin") || strings.Contains(route.Path, "membership") {
			t.Fatalf("private route registered: %s", route.Path)
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
		if strings.Contains(name, "admin") || strings.Contains(name, "membership") || strings.Contains(name, "invitation") {
			t.Fatalf("private table created: %s", name)
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
