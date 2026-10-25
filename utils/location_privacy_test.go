package utils

import (
	"database/sql"
	"testing"
)

func TestRepositoryDoesNotPersistOrRestoreCoordinates(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = createTables(db); err != nil {
		t.Fatal(err)
	}
	r := NewUserRepository(db)
	if err = r.SaveUser("u", "password", "token", "23.123456", "113.654321", "", "", "", 1, "", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err = r.UpdateCoordinates("u", "24.123456", "114.654321"); err != nil {
		t.Fatal(err)
	}
	var lat, lon string
	if err = db.QueryRow("SELECT latitude,longitude FROM users WHERE account='u'").Scan(&lat, &lon); err != nil {
		t.Fatal(err)
	}
	if lat != "" || lon != "" {
		t.Fatal("repository persisted coordinates")
	}
	if _, err = db.Exec("UPDATE users SET latitude='23',longitude='113'"); err != nil {
		t.Fatal(err)
	}
	_, lat, lon, err = r.GetUser("u")
	if err != nil || lat != "" || lon != "" {
		t.Fatal("repository restored legacy coordinates")
	}
	if err = r.SaveUser("u", "new-password", "new-token", "24", "114", "", "", "", 1, "", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT latitude,longitude FROM users WHERE account='u'").Scan(&lat, &lon); err != nil || lat != "23" || lon != "113" {
		t.Fatal("historical data changed without authorization")
	}
}
