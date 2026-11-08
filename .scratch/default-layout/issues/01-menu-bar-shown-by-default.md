# 01: 菜单栏默认显示

**What to build:** 没有设置过菜单栏的用户，在 Windows 和 Linux 上启动 Mayoi 时窗口顶部直接显示 File 到 Help 的菜单栏；显式关掉过菜单栏的用户重启后仍然不显示；macOS 继续使用系统菜单栏，窗口内不出现菜单条。

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] `menuBarExpanded` 未设置时按显示处理，显式 `false` 时保持隐藏
- [x] 用 View 菜单或命令面板切换后写入配置文件，重启保持
- [x] 旧配置文件不需要手动迁移，键名与 `settings.json` 的 schema 条目不变
- [x] 设置文档补一句默认显示
- [x] 标题栏相关测试覆盖三种情况：缺省、显式关闭、显式打开

## Answer

`MenuBarExpanded` 由 `bool` 改为 `*bool`，`Prefs.MenuBarShown()` 封装"nil 即显示"，`Default()` 预置 `&true`。`toggleMenuBar` / `resetLayout` 通过 `SetMenuBarShown` 写值，`omitempty` 使显式 `false` 落盘。`docs/SETTINGS.md` 默认值改为 `true`。`settings_test.go` 覆盖 nil/false/true 三态。
