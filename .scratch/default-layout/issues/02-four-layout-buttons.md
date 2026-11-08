# 02: 标题栏统一的四个布局图标

**What to build:** 标题栏最右侧依次排四个图标：自定义布局、侧边栏、面板、停靠右。原来跟随侧边栏和面板位置分散摆放的按钮都被这四个替代。点侧边栏和面板图标分别显示/隐藏侧边栏和面板，点停靠右图标把侧边栏在左右之间移动，图标方向随停靠方向变化。

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] 四个图标固定在标题栏最右侧，顺序与截图一致，窗口变窄时不被挤出窗口
- [x] 侧边栏图标与 `Ctrl+B` 行为一致，面板图标与 `Ctrl+`` ` 行为一致
- [x] 面板关闭再打开时回到上次显示的内容（终端或问题），首次打开默认终端
- [x] 停靠右图标点击后侧边栏位置与图标方向同时变化
- [x] Windows、Linux、macOS 都显示四个图标，菜单栏仍只在非 macOS 显示
- [x] 标题栏顺序与点击行为的测试通过，原有标题栏测试相应更新

## Answer

`titleBar` 右侧统一为 `layoutButtons`：自定义布局（新 `iconLayout`）、侧边栏、面板、停靠右。`togglePanel` 不再在打开时强制 `panelProblems=false`，改为按上次内容恢复。停靠右图标随 `SidebarRight` 在 `iconSidebar`/`iconSidebarRight` 间切换。右行 `MinWidth(112)` 防挤出。更新 `TestTerminalPanelAtASide`，新增 `TestTitleBarLayoutButtons`。
