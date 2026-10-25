package utils

import (
	"bytes"
	"database/sql"
	"path/filepath"
	"testing"
	"xixunyunsign/service"
)

func TestSessionCredentialsSurviveReopenWithoutPlaintextAndCannotMoveAccounts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = createTables(db); err != nil {
		t.Fatal(err)
	}
	repo := NewUserRepository(db)
	store, ok := repo.(service.SessionCredentials)
	if !ok {
		t.Fatal("repository cannot persist renewal credentials")
	}
	if err = store.SaveSessionCredentials("001123", "unique-school-password", "12"); err != nil {
		t.Fatal(err)
	}
	var sealed []byte
	if err = db.QueryRow("SELECT protected FROM upstream_credentials WHERE account='001123'").Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("unique-school-password")) {
		t.Fatal("plaintext password persisted")
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = NewUserRepository(db).(service.SessionCredentials)
	password, school, err := store.SessionCredentials("001123")
	if err != nil || password != "unique-school-password" || school != "12" {
		t.Fatal("renewal credentials lost on restart", err)
	}
	if _, err = db.Exec("INSERT INTO upstream_credentials(account,protected) VALUES('other',?)", sealed); err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.SessionCredentials("other"); err == nil {
		t.Fatal("encrypted credential transplanted to another account")
	}
	sealed[len(sealed)-1] ^= 1
	if _, err = db.Exec("UPDATE upstream_credentials SET protected=? WHERE account='001123'", sealed); err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.SessionCredentials("001123"); err == nil {
		t.Fatal("corrupt credentials accepted")
	}
}
