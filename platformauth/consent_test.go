package platformauth

import "testing"

func TestConsentPersistsPerUserAndVersionAndKeepsOriginalTime(t *testing.T) {
	s, _ := testStore(t)
	u, _, err := s.LoginOfficial("001123", "1", "学校", "", "fixture", func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecordConsent(u.ID); err != nil {
		t.Fatal(err)
	}
	var first int64
	if err = s.db.QueryRow("SELECT accepted_at FROM platform_consents WHERE user_id=? AND version=?", u.ID, AgreementVersion).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if first == 0 {
		t.Fatal("confirmation time missing")
	}
	if err = s.RecordConsent(u.ID); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(s.db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if accepted, err := reopened.HasAccountConsent("001123"); err != nil || !accepted {
		t.Fatal("consent not restored", err)
	}
	if accepted, err := reopened.HasAccountConsent("002456"); err != nil || accepted {
		t.Fatal("consent shared across users", err)
	}
	var repeated int64
	if err = s.db.QueryRow("SELECT accepted_at FROM platform_consents WHERE user_id=? AND version=?", u.ID, AgreementVersion).Scan(&repeated); err != nil || repeated != first {
		t.Fatal("confirmation time overwritten", err)
	}
	if _, err = s.db.Exec("UPDATE platform_consents SET version='old-version' WHERE user_id=?", u.ID); err != nil {
		t.Fatal(err)
	}
	if accepted, err := reopened.HasConsent(u.ID); err != nil || accepted {
		t.Fatal("outdated agreement treated as current", err)
	}
}
