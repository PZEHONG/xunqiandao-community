package platformauth

import "database/sql"

func ProvideStore(db *sql.DB) (*Store, error) { return New(db, SMTPMailer{}) }
