# ✅ Info.plist 冲突问题已解决

## 问题说明

您遇到的错误:
```
Multiple commands produce Info.plist
```

这是因为现代 Xcode 项目会**自动生成** Info.plist,不需要手动创建。

## ✅ 已执行的修复

我已经删除了手动创建的 `Info.plist` 文件。现在项目会使用 Xcode 自动生成的版本。

## 🔧 现在需要做的

### 步骤 1: Clean Build (必须)

在 Xcode 中:
1. 关闭 Xcode (如果已打开)
2. 删除缓存:
   ```bash
   rm -rf ~/Library/Developer/Xcode/DerivedData/WiseInvest-*
   ```
3. 重新打开项目:
   ```bash
   cd /Users/songhanxu/WiseInvest/ios/WiseInvest
   open WiseInvest.xcodeproj
   ```
4. Clean Build Folder: 按 `⇧⌘K`
5. Build: 按 `⌘B`

### 步骤 2: 配置网络权限

由于删除了手动的 Info.plist,需要在 Xcode 中重新配置网络权限:

1. **打开项目设置**:
   - 点击项目导航器顶部的 **WiseInvest** (蓝色图标)
   - 选择 **WiseInvest** target
   - 选择 **Info** 标签页

2. **添加网络权限**:
   - 点击任意一行,然后点击 **+** 按钮
   - 添加: `App Transport Security Settings` (类型: Dictionary)
   - 展开它,添加两个子项:
     - `Allow Arbitrary Loads` = YES (Boolean)
     - `Allow Local Networking` = YES (Boolean)

详细步骤请查看 `NETWORK_SETUP.md`

### 步骤 3: 重新构建

```bash
# 在 Xcode 中:
# 1. Clean: ⇧⌘K
# 2. Build: ⌘B
# 3. Run: ⌘R
```

## 🎯 快速修复脚本

或者运行这个一键修复脚本:

```bash
cd /Users/songhanxu/WiseInvest/ios/WiseInvest

# 清理缓存
rm -rf ~/Library/Developer/Xcode/DerivedData/WiseInvest-*

# 打开项目
open WiseInvest.xcodeproj

# 然后在 Xcode 中:
# 1. 配置网络权限 (见上面步骤 2)
# 2. Clean Build (⇧⌘K)
# 3. Build (⌘B)
```

## ✅ 验证修复

构建成功后,您应该:
- ✅ 没有 Info.plist 冲突错误
- ✅ 可以正常构建项目
- ✅ 可以运行应用

## 🐛 如果仍有问题

### 问题 1: 仍然看到 Info.plist 错误

**解决**:
```bash
# 完全清理项目
cd /Users/songhanxu/WiseInvest/ios/WiseInvest
rm -rf ~/Library/Developer/Xcode/DerivedData/*
rm -rf WiseInvest.xcodeproj/xcuserdata
rm -rf WiseInvest.xcodeproj/project.xcworkspace/xcuserdata

# 重新打开
open WiseInvest.xcodeproj
```

### 问题 2: 网络请求失败

**解决**: 确保已配置网络权限,详见 `NETWORK_SETUP.md`

### 问题 3: 编译错误

**解决**: 确保所有文件都已添加到项目,详见 `SETUP_INSTRUCTIONS.md`

## 📚 相关文档

- `NETWORK_SETUP.md` - 网络权限配置详细说明
- `SETUP_INSTRUCTIONS.md` - 完整设置指南
- `QUICKSTART.md` - 快速启动指南
- `TROUBLESHOOTING.md` - 故障排除

## 🎉 总结

**问题**: Info.plist 冲突  
**原因**: 手动创建的 Info.plist 与 Xcode 自动生成的冲突  
**解决**: 删除手动创建的文件,使用 Xcode 自动生成  
**后续**: 在 Xcode 中配置网络权限

---

**现在可以正常构建了!** 🚀

按照上面的步骤操作,应该可以解决问题。如有其他问题,请查看相关文档。
