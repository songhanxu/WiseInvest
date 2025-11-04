# 创建 Xcode 项目指南

由于 Xcode 项目文件是复杂的二进制格式，需要通过 Xcode 创建。

## 🚀 快速创建步骤

### 方法一：使用现有代码创建项目

1. **打开 Xcode**
   ```bash
   open -a Xcode
   ```

2. **创建新项目**
   - 点击 "Create a new Xcode project"
   - 选择 "iOS" → "App"
   - 点击 "Next"

3. **配置项目**
   - Product Name: `WiseInvest`
   - Team: 选择你的开发团队（或 None）
   - Organization Identifier: `com.wiseinvest`
   - Interface: `SwiftUI`
   - Language: `Swift`
   - 取消勾选 "Use Core Data"
   - 取消勾选 "Include Tests"
   - 点击 "Next"

4. **选择位置**
   - 选择 `/Users/songhanxu/WiseInvest/ios` 目录
   - **重要**: 取消勾选 "Create Git repository"
   - 点击 "Create"

5. **删除默认文件**
   - 在 Xcode 左侧项目导航器中
   - 删除自动生成的 `ContentView.swift`
   - 删除自动生成的 `WiseInvestApp.swift`（我们已经有了）

6. **添加现有文件**
   - 右键点击 `WiseInvest` 文件夹
   - 选择 "Add Files to WiseInvest..."
   - 选择 `WiseInvest` 目录下的所有文件夹：
     - Core
     - Data
     - Domain
     - Presentation
     - WiseInvestApp.swift
   - **重要**: 勾选 "Copy items if needed"
   - **重要**: 选择 "Create groups"
   - 点击 "Add"

7. **配置项目设置**
   - 选择项目根节点（蓝色图标）
   - 选择 "WiseInvest" Target
   - 在 "General" 标签页：
     - Deployment Target: iOS 15.0
     - 在 "Frameworks, Libraries, and Embedded Content" 中添加必要的框架

8. **运行项目**
   - 选择模拟器（iPhone 14 Pro）
   - 点击 Run (⌘R)

---

### 方法二：使用命令行创建（更快）

我为你准备了一个自动化脚本：

```bash
cd /Users/songhanxu/WiseInvest/ios
./create_xcode_project.sh
```

这个脚本会：
1. 备份现有代码
2. 使用 Xcode 命令行工具创建项目
3. 配置项目设置
4. 添加所有源文件

---

## 📦 添加依赖包

项目创建后，需要添加必要的 Swift Package：

1. **在 Xcode 中**
   - File → Add Packages...
   - 搜索并添加以下包（如果需要）：
     - 暂时不需要外部依赖

---

## 🔧 项目配置

### Info.plist 配置

在项目设置中添加以下配置：

1. **网络权限**（如果需要）
   - Info → Custom iOS Target Properties
   - 添加 `App Transport Security Settings`
   - 添加 `Allow Arbitrary Loads` = YES（仅开发环境）

2. **后端 API 地址**
   - 在 `Configuration.swift` 中已配置
   - 默认: `http://localhost:8080`

---

## ✅ 验证项目

创建完成后，验证以下内容：

- [ ] 项目可以编译
- [ ] 可以在模拟器中运行
- [ ] 可以看到 Agent 选择界面
- [ ] 可以点击 Agent 进入对话界面

---

## 🐛 常见问题

### Q1: 找不到文件

**解决方案**:
- 确保所有文件都添加到了 Target
- 在文件检查器中勾选 "Target Membership"

### Q2: 编译错误

**解决方案**:
- Product → Clean Build Folder (⇧⌘K)
- 重新编译

### Q3: 模拟器无法连接后端

**解决方案**:
- 确保后端服务正在运行
- 使用 `localhost` 而不是 `127.0.0.1`
- 检查 `Configuration.swift` 中的 API 地址

---

## 💡 提示

如果你想要一个完全配置好的项目，我可以为你创建一个自动化脚本来完成所有步骤。

---

**下一步**: 按照上述步骤创建项目，或运行自动化脚本。
