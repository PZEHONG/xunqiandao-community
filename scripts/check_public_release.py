from pathlib import Path
import re, sys, json, subprocess
root=Path(sys.argv[1]).resolve()
errors=[]
# Check exactly the files Git can publish, including force-added ignored files.
# Local test traces, installed dependencies and compiled bundles are not releases.
if (root/'.git').exists():
    names=subprocess.check_output(['git','ls-files','-z','--cached','--others','--exclude-standard'],cwd=root).decode().split('\0')
    paths=[root/name for name in sorted(set(names)) if name]
else:
    paths=[p for p in root.rglob('*') if p.is_file() and not any(x in p.relative_to(root).parts for x in ('.git','node_modules','berry','test-results','playwright-report','coverage'))]
for p in paths:
    if not p.is_file(): continue
    rel=p.relative_to(root).as_posix()
    if re.search(r'(^|/)(admin|membership|recharge|alipay|invitation)(/|[._])',rel,re.I): errors.append((rel,'private feature path'))
    if p.suffix.lower() in ('.db','.sqlite','.sqlite3','.pem','.key','.p12','.pfx','.exe','.dll','.zip','.log','.http','.har','.bak','.tgz','.gz','.7z') or re.search(r'\.(?:db|sqlite\d?)-(?:wal|shm|journal)$',p.name,re.I) or (p.name.startswith('.env') and p.name!='.env.example'): errors.append((rel,'forbidden runtime or secret file'))
    if p.suffix not in ('.go','.ts','.tsx','.js','.json','.html','.css'): continue
    value=p.read_text(encoding='utf-8')
    for pattern,kind in [(r'xixunyunsign/(?:membership|alipay)|features/membership|register(?:Membership|Admin|Recharge|Announcement)Routes|initAdmin\(|membershipWriteTx|rewardInvitation|platform_user_controls|auto_sign_admin_controls|entitlement_hold','private implementation reference'),(r'-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----|gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{40,}|AKIA[A-Z0-9]{16}|sk-[A-Za-z0-9_-]{20,}','secret marker')]:
        if re.search(pattern,value): errors.append((rel,kind))
    if rel.startswith('frontend/src/') and re.search(r'REPORT_AI_API_KEY|SMTP_PASSWORD|GEETEST_API_KEY|AMAP_WEB_KEY|AMAP_SECURITY_JS_CODE',value): errors.append((rel,'server credential referenced by browser code'))
print(json.dumps({'files_checked':len(paths),'findings':errors},ensure_ascii=False,indent=2))
sys.exit(bool(errors))
