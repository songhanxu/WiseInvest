# 📱 iOS 项目简易设置指南

## 🎯 最简单的方法

### 步骤 1: 打开 Xcode

```bash
open -a Xcode
```

### 步骤 2: 创建新项目

1. 点击 **"Create a new Xcode project"**
2. 选择 **iOS** → **App**
3. 点击 **Next**

### 步骤 3: 配置项目

填写以下信息：

| 字段 | 值 |
|------|-----|
| Product Name | `WiseInvest` |
| Team | 选择你的团队（或 None） |
| Organization Identifier | `com.wiseinvest` |
| Bundle Identifier | `com.wiseinvest.WiseInvest` |
| Interface | **SwiftUI** ⚠️ 重要 |
| Language | **Swift** |
| Storage | None |
| Include Tests | 取消勾选 |

点击 **Next**

### 步骤 4: 选择保存位置

1. 导航到: `/Users/songhanxu/WiseInvest/ios`
2. **取消勾选** "Create Git repository"
3. 点击 **Create**

### 步骤 5: 清理默认文件

Xcode 会自动创建一些文件，我们需要删除它们：

1. 在左侧项目导航器中，找到并删除：
   - `ContentView.swift` （右键 → Delete → Move to Trash）
   - 自动生成的 `WiseInvestApp.swift` （右键 → Delete → Move to Trash）

### 步骤 6: 添加我们的代码

1. 在 Finder 中打开 `/Users/songhanxu/WiseInvest/ios/WiseInvest` 目录
2. 将以下文件夹拖到 Xcode 左侧的项目导航器中（拖到 WiseInvest 文件夹下）：
   - `Core` 文件夹
   - `Data` 文件夹
   - `Domain` 文件夹
   - `Presentation` 文件夹
   - `WiseInvestApp.swift` 文件

3. 在弹出的对话框中：
   - ✅ 勾选 **"Copy items if needed"**
   - ✅ 选择 **"Create groups"**
   - ✅ 确保 **"Add to targets: WiseInvest"** 被勾选
   - 点击 **Finish**

### 步骤 7: 配置项目设置

1. 点击左侧最上方的蓝色项目图标
2. 选择 **WiseInvest** Target（不是 Project）
3. 在 **General** 标签页：
   - **Deployment Info** → **iOS Deployment Target**: 选择 `15.0`
   - **Supported Destinations**: 只勾选 `iPhone`

### 步骤 8: 运行项目

1. 在顶部工具栏选择模拟器：
   - 点击设备选择器（默认显示 "Any iOS Device"）
   - 选择 **iPhone 14 Pro** 或其他 iPhone 模拟器

2. 点击 **Run** 按钮（▶️）或按 `⌘R`

3. 等待编译和模拟器启动

4. 应该能看到 WiseInvest 的主界面！

---

## 🐛 可能遇到的问题

### 问题 1: 找不到文件

**症状**: 编译时提示找不到某些文件

**解决方案**:
1. 选中文件
2. 在右侧 **File Inspector** 中
3. 确保 **Target Membership** 中 `WiseInvest` 被勾选

### 问题 2: 编译错误

**症状**: 各种编译错误

**解决方案**:
1. Product → Clean Build Folder (`⇧⌘K`)
2. 重新编译 (`⌘B`)

### 问题 3: 模拟器启动失败

**解决方案**:
1. Xcode → Preferences → Locations
2. 确保 Command Line Tools 已选择
3. 重启 Xcode

### 问题 4: 无法连接后端

**症状**: 应用显示网络错误

**解决方案**:
1. 确保后端服务正在运行：
   ```bash
   curl http://localhost:8080/health
   ```

2. 检查 `Core/Config/Configuration.swift` 中的 API 地址
   - 应该是 `http://localhost:8080`
   - 不要使用 `127.0.0.1`

---

## ✅ 验证清单

创建完成后，检查以下内容：

- [ ] 项目可以编译（无错误）
- [ ] 可以在模拟器中运行
- [ ] 可以看到两个 Agent 卡片（绿色和蓝色）
- [ ] 点击 Agent 可以进入对话界面
- [ ] 可以输入消息（虽然后端可能还没运行）

---

## 📸 预期效果

### 主界面
- 标题: "慧投 WiseInvest"
- 两个 Agent 卡片:
  - 🤖 投资顾问（绿色）
  - 💹 交易助手（蓝色）
- 最近对话列表

### 对话界面
- 顶部显示 Agent 名称
- 消息列表
- 底部输入框

---

## 🎨 自定义配置（可选）

### 修改应用图标

1. 在 Assets.xcassets 中
2. 点击 AppIcon
3. 拖入不同尺寸的图标

### 修改启动画面

1. 在 Assets.xcassets 中
2. 添加 LaunchScreen
3. 配置启动画面

### 修改 API 地址

编辑 `Core/Config/Configuration.swift`:

```swift
static let baseURL = "http://your-server:8080"
```

---

## 💡 提示

1. **首次编译会比较慢** - Xcode 需要索引所有文件
2. **使用快捷键** - `⌘B` 编译，`⌘R` 运行，`⌘.` 停止
3. **查看日志** - 在底部的控制台可以看到 print 输出
4. **使用断点** - 点击行号左侧可以设置断点调试

---

## 📚 下一步

项目创建成功后：

1. **启动后端服务**:
   ```bash
   cd ../backend
   ./start.sh
   ```

2. **测试对话功能**:
   - 在模拟器中点击"投资顾问"
   - 输入消息测试

3. **查看日志**:
   - 后端日志: `tail -f logs/backend.log`
   - iOS 日志: Xcode 底部控制台

---

**预计时间**: 5-10 分钟

**难度**: ⭐⭐☆☆☆ (简单)

如有问题，请查看 [CREATE_XCODE_PROJECT.md](CREATE_XCODE_PROJECT.md) 获取更详细的说明。
