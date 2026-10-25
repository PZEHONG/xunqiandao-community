package platformauth

import "strings"

// AgreementVersion changes when the displayed agreement texts need new consent.
const AgreementVersion = "2026-10-16-auto-renew"

func (s *Store) HasAccountConsent(account string) (bool, error) {
	var accepted bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM platform_consents c JOIN platform_users u ON u.id=c.user_id WHERE u.username=? AND c.version=?)`, strings.TrimSpace(account), AgreementVersion).Scan(&accepted)
	return accepted, err
}

func (s *Store) HasConsent(userID int64) (bool, error) {
	var accepted bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM platform_consents WHERE user_id=? AND version=?)`, userID, AgreementVersion).Scan(&accepted)
	return accepted, err
}

// RecordConsent is called only after successful authentication. Repeated login
// does not replace the time of the user's original explicit confirmation.
func (s *Store) RecordConsent(userID int64) error {
	_, err := s.db.Exec(`INSERT INTO platform_consents(user_id,version,accepted_at) VALUES(?,?,?) ON CONFLICT(user_id,version) DO NOTHING`, userID, AgreementVersion, s.now().Unix())
	return err
}
