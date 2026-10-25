# 讯签到 · 习讯云实习签到工作台

## 正在用习讯云？打开网站，直接开始

### [👉 立即使用讯签到 · https://xqd.aist.ink](https://xqd.aist.ink)

**实习里的小事，也值得被好好安排。**

把习讯云签到、每日计划和实习报告放在一个工作台：地图选择签到地点，按需设置每日签到时间，随时回看签到记录、实习周报和月报。电脑、手机浏览器均可访问。

**[前往签到](https://xqd.aist.ink/#/sign) · [设置每日计划](https://xqd.aist.ink/#/sign?mode=daily) · [查看实习周报](https://xqd.aist.ink/#/reports/week)**

[![讯签到功能预览：异地签到、每日自动签到与实习报告](docs/website-preview.png)](https://xqd.aist.ink)

> 希望直接使用？访问 **[xqd.aist.ink](https://xqd.aist.ink)**，无需自行安装和维护服务器。在线服务的功能与权益以网站说明为准；上图为在线网站展示。

---

## 社区开源版

这是讯签到的社区源码仓库，供学习、研究和自行部署。它是面向习讯云的独立第三方辅助工具，与习讯云官方无隶属关系。

| 功能 | 社区版内容 |
| --- | --- |
| 习讯云账号连接 | 学校搜索、学号验证、账号绑定与会话恢复 |
| 签到记录 | 按月查询、日历展示、官方记录核对 |
| 地图选点签到 | 地点搜索、地址确认、备注与照片上传 |
| 每日签到计划 | 时间配置、暂停/恢复、编辑/删除、执行历史 |
| 实习报告 | 周报/月报列表、详情、草稿、附件与提交 |
| 响应式界面 | React + TypeScript，兼容电脑与手机 |

**会员中心及其权益、兑换码、邀请奖励、充值等实现，以及管理端页面、接口和数据模型不在本仓库发布范围内。** 社区版的每日计划可在自行部署的服务上独立运行，不依赖在线网站的会员系统。日报功能尚未实现。

## 技术与环境

- Go：版本以 [go.mod](go.mod) 为准（当前 `1.27.0`，工具链 `1.27.3`）。
- SQLite：使用 `mattn/go-sqlite3`，需要开启 CGO 并安装 C/C++ 编译工具链；Windows 可使用 MinGW-w64。
- Node.js：`20.19+` 或 `22.12+`，建议与 CI 一致使用 Node.js 22。
- React 19、TypeScript、Vite、MUI；后端使用 Gin、Cobra。

## 本地运行

```bash
git clone https://github.com/PZEHONG/xunqiandao-community.git
cd xunqiandao-community
```

将 `.env.example` 复制为 `.env`，自行申请并填写地图和人机验证配置。模板不包含任何真实凭证；请勿提交你的 `.env`、数据库或加密密钥。

```bash
# 构建前端
cd frontend
npm ci
npm run build
cd ..

# 构建后端（需要 C/C++ 编译器，CGO_ENABLED=1）
go build -trimpath -o output/xunqiandao main.go

# 启动本地服务
./output/xunqiandao serve --host 127.0.0.1 --port 8080
```

Windows 编译可将输出文件名改为 `output/xunqiandao.exe`。程序从**启动时的工作目录**读取 `.env` 并创建本地 `config.db`，请固定工作目录并妥善保存运行数据。

打开 [http://127.0.0.1:8080](http://127.0.0.1:8080)，选择学校并使用自己的习讯云账号登录。`/api/health` 提供健康检查；`/legacy/` 保留旧版界面入口。

开发前端时，可在已启动后端的情况下于 `frontend` 运行 `npm run dev`，开发服务会代理本机 8080 端口的 API。

## 配置说明

| 配置 | 用途 |
| --- | --- |
| `AMAP_WEB_KEY` | 地点搜索和逆地理编码，使用高德 Web 服务 Key |
| `AMAP_JS_KEY` | 高德地图 JS API Key |
| `AMAP_SECURITY_JS_CODE` | JS API 对应的安全密钥，由服务端代理使用 |
| `AMAP_DEFAULT_CITY` | 可选的默认搜索城市 |
| `GEETEST_CAPTCHA_ID` / `GEETEST_API_KEY` | 极验 GT4.0 人机验证配置 |
| `GEETEST_ENABLED` | 默认启用；仅本机隔离测试可设为 `false` |

SMTP 参数仅供保留的内部邮件组件使用；当前网页不提供邮箱注册、找回密码入口。对外部署还需配置 HTTPS、可信反向代理和持久化存储，见 [.env.example](.env.example) 中的说明。

## 每日计划如何执行

1. 连接账号并确认学校，选择地点、签到类型和时间。
2. 核对任务内容后明确确认启用，从下一个计划时刻开始执行。
3. 任务按北京时间每天运行（包含周末）；同一任务同一日期最多提交一次。
4. 任务在**你部署的服务端**执行。关闭网页不影响任务，但运行服务的电脑或服务器必须开机联网；服务停止、休眠期间错过的时刻不补签。
5. 遇到结果不明确、执行中断或失败时，先核对习讯云官方记录，系统不会自动重试签到提交。

## 检查与测试

```bash
python scripts/check_public_release.py .
go test ./...
go vet ./...

cd frontend
npm run lint
npm test
npm run build
```

仓库还保留了适用的 Playwright 测试和模拟数据。端到端测试需要本机 Chrome，运行 `npm run test:e2e` 时会自动启动独立前端开发服务。测试数据均应为虚构数据，不要把真实账号放进测试文件。

## 源码结构

```text
frontend/       React 工作台与组件测试
web/            HTTP API、会话检查、地点确认、旧版静态资源
platformauth/   普通用户登录、绑定、人机验证、会话
autosign/       每日计划、执行调度与历史记录
service/        习讯云业务接口与实现
utils/          SQLite、账号存储、凭证保护
cmd/            命令行功能
internal/wire/  依赖装配
scripts/        开源发布范围检查
```

## 安全与使用边界

请仅使用自己的账号，遵守学校、实习单位与平台的签到要求。签到和报告是否有效，以习讯云官方记录及学校要求为准。

项目不调用浏览器设备定位，使用用户确认的候选地点。密码等续期凭证由本地加密存储保护，数据库、会话文件、图片附件仍需由部署者妥善保管。不要在 Issue、截图、日志或 Pull Request 中公开账号、Cookie、Token、密钥或个人资料。发现安全问题请参考 [SECURITY.md](SECURITY.md)。

## 许可证与致谢

沿用项目已有的 [GNU GPL v3.0](LICENSE) 许可证。Berry Free React 模板相关部分保留 MIT 许可证，见 [第三方来源](frontend/THIRD_PARTY.md) 与 [Berry 许可证](frontend/licenses/berry-LICENSE)。

觉得有帮助，欢迎 Star、反馈问题和贡献改进。也欢迎直接使用 **[讯签到在线工作台](https://xqd.aist.ink)**。
