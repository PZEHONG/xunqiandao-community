# 第三方来源

Berry Free 官方仓库：https://github.com/codedthemes/berry-free-react-admin-template

固定 commit：`c5709e34ceb162eb2b46ad55f16464aa8b0794b3`。MIT 原文保留于 `licenses/berry-LICENSE`。

复用/改编清单：

| 上游文件 | 本项目 | 改动 |
| --- | --- | --- |
| `vite/src/ui-component/cards/MainCard.jsx` | `src/components/MainCard.tsx` | TypeScript；保留 Card/Header/Divider/Content 结构，收窄为实际使用属性 |
| `vite/src/themes/index.jsx` | `src/theme/index.ts` | 保留 createTheme/ThemeProvider/CssBaseline 组合；仅浅色，原品牌色/系统字体/控件尺寸 |
| `vite/src/layout/MainLayout/index.jsx`、`MainContentStyled.jsx`、`Sidebar/index.jsx` | `src/layout/*` | 参考 AppBar/侧栏/圆角内容区结构，以 CSS 侧栏适配原断点；移除示例页、图表、配置器、页脚；增加760px底部五项导航 |

不是完整 Berry 示例工程；不使用 Pro 代码或模板认证服务。业务代码使用 TypeScript。

上游 vite/package.json 实际版本：React/DOM 19.2.0、MUI/icons 7.3.5、Emotion 11.14.0/11.14.1、Router 7.9.6、Vite 7.2.6、React 插件5.1.1。没有 engines 字段。Vite 7.2.6 的 npm 元数据要求 Node `^20.19.0 || >=22.12.0`；当前 Node22.14.0、npm10.9.2 满足。MUI支持 React17/18/19，Emotion peer `^11.5.0/^11.3.0`。未选图表、动画、网络字体等依赖。

使用一个 npm package-lock.json；package.json 直接依赖固定精确版本。构建工具、测试库版本也固定于该文件。

## 本地安全修补（2026-10-18）

保留上述Berry上游固定commit与许可证；本地锁定Router7.18.4、Vite7.3.7、Vitest4.1.12修复审计告警。上游原始版本信息仅为来源记录，实际安装以package.json/package-lock.json为准。npm10 ci及新版完整回归独立验证；没有复制Berry Pro代码。
