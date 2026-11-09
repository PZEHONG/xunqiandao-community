from pathlib import Path
import json
import subprocess
import sys
import tempfile
import unittest

CHECKER = Path(__file__).with_name('check_public_release.py')


class PublicReleaseBoundaryTests(unittest.TestCase):
    def scan(self, files):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            for name, content in files.items():
                target = root / name
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_text(content, encoding='utf-8')
            result = subprocess.run([sys.executable, str(CHECKER), str(root)], capture_output=True, text=True, encoding='utf-8', check=False)
            return result.returncode, json.loads(result.stdout)['findings']

    def test_public_ai_and_notifications_are_allowed(self):
        code, findings = self.scan({'web/report_ai.go': 'package web\n', 'platformauth/notification.go': 'package platformauth\n', '.env.example': 'REPORT_AI_API_KEY=\nSMTP_PASSWORD=\n'})
        self.assertEqual((code, findings), (0, []))

    def test_closed_features_are_blocked(self):
        for name in ('web/recharge.go', 'alipay/client.go', 'platformauth/membership_trial.go', 'frontend/src/admin/App.tsx'):
            with self.subTest(name=name):
                code, findings = self.scan({name: ''})
                self.assertNotEqual(code, 0)
                self.assertTrue(any(kind == 'private feature path' for _, kind in findings))

    def test_database_sidecars_and_environment_files_are_blocked(self):
        for name in ('config.db-wal', 'config.db-shm', '.env.production', 'snapshot.tgz'):
            with self.subTest(name=name):
                self.assertNotEqual(self.scan({name: ''})[0], 0)

    def test_browser_credentials_are_blocked(self):
        code, findings = self.scan({'frontend/src/config.ts': 'const credential = import.meta.env.REPORT_AI_API_KEY;'})
        self.assertNotEqual(code, 0)
        self.assertTrue(any(kind == 'server credential referenced by browser code' for _, kind in findings))

    def test_secret_markers_are_blocked(self):
        self.assertNotEqual(self.scan({'web/config.go': 'var credential = "' + 'sk-' + 'x' * 30 + '"'})[0], 0)


if __name__ == '__main__':
    unittest.main()
