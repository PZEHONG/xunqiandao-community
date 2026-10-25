# 安全问题

请不要在公开 Issue 或 Pull Request 中提交真实账号、密码、Cookie、Token、数据库、私密日志或密钥。

如发现漏洞，请使用仓库的 Security 页面中的私密漏洞报告入口。若入口不可用，可以先提交不含漏洞细节和敏感数据的 Issue，请求维护者提供私密联系方式。

本仓库不接受运行数据、生产环境配置、会员系统或管理端源码。提交前运行 `python scripts/check_public_release.py .`，并使用 Gitleaks 等工具扫描暂存内容及 Git 历史。
