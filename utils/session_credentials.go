package utils

import (
	"encoding/json"
	"errors"
	"path/filepath"
)

type sessionCredential struct {
	Password string `json:"password"`
	SchoolID string `json:"school_id"`
}

func (r *UserRepositoryImpl) sessionKeyPath() (string, error) {
	var sequence int
	var name, path string
	err := r.db.QueryRow("PRAGMA database_list").Scan(&sequence, &name, &path)
	if err != nil || path == "" {
		return "", errors.New("persistent credential storage unavailable")
	}
	return filepath.Clean(path) + ".session-key", nil
}

func (r *UserRepositoryImpl) SaveSessionCredentials(account, password, schoolID string) error {
	if account == "" || password == "" || schoolID == "" {
		return errors.New("missing renewal credentials")
	}
	payload, err := json.Marshal(sessionCredential{Password: password, SchoolID: schoolID})
	if err != nil {
		return err
	}
	keyPath, err := r.sessionKeyPath()
	if err != nil {
		return err
	}
	sealed, err := protectSessionCredential(payload, []byte(account), keyPath)
	if err != nil {
		return err
	}
	_, err = r.db.Exec("INSERT INTO upstream_credentials(account,protected) VALUES(?,?) ON CONFLICT(account) DO UPDATE SET protected=excluded.protected", account, sealed)
	return err
}

func (r *UserRepositoryImpl) SessionCredentials(account string) (string, string, error) {
	var sealed []byte
	if err := r.db.QueryRow("SELECT protected FROM upstream_credentials WHERE account=?", account).Scan(&sealed); err != nil {
		return "", "", err
	}
	keyPath, err := r.sessionKeyPath()
	if err != nil {
		return "", "", err
	}
	payload, err := unprotectSessionCredential(sealed, []byte(account), keyPath)
	if err != nil {
		return "", "", errors.New("renewal credentials unavailable")
	}
	var credential sessionCredential
	if json.Unmarshal(payload, &credential) != nil || credential.Password == "" || credential.SchoolID == "" {
		return "", "", errors.New("invalid renewal credentials")
	}
	return credential.Password, credential.SchoolID, nil
}
