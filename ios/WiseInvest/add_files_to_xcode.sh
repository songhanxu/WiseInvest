#!/bin/bash

# Script to help add files to Xcode project
# This script provides instructions and verification

set -e

PROJECT_DIR="/Users/songhanxu/WiseInvest/ios/WiseInvest"
PROJECT_FILE="$PROJECT_DIR/WiseInvest.xcodeproj"

echo "🔧 WiseInvest - Xcode 文件添加助手"
echo "=================================="
echo ""

# Check if project exists
if [ ! -d "$PROJECT_FILE" ]; then
    echo "❌ 错误: 找不到 Xcode 项目"
    echo "   路径: $PROJECT_FILE"
    exit 1
fi

echo "✅ 找到 Xcode 项目"
echo ""

# List all Swift files that need to be added
echo "📋 需要添加到项目的文件:"
echo ""

find "$PROJECT_DIR/WiseInvest" -name "*.swift" -type f | while read file; do
    rel_path=${file#$PROJECT_DIR/WiseInvest/}
    if [[ "$rel_path" != "WiseInvestApp.swift" ]]; then
        echo "   ✓ $rel_path"
    fi
done

echo ""
echo "📁 文件夹结构:"
echo ""
echo "   WiseInvest/"
echo "   ├── Core/"
echo "   │   ├── Coordinator/"
echo "   │   └── Extensions/"
echo "   ├── Data/"
echo "   │   ├── Network/"
echo "   │   └── Repository/"
echo "   ├── Domain/"
echo "   │   ├── Models/"
echo "   │   └── Repository/"
echo "   └── Presentation/"
echo "       ├── Home/"
echo "       ├── Conversation/"
echo "       └── Components/"
echo ""

# Count files
SWIFT_FILES=$(find "$PROJECT_DIR/WiseInvest" -name "*.swift" -type f | wc -l | tr -d ' ')
echo "📊 统计: 共 $SWIFT_FILES 个 Swift 文件"
echo ""

echo "🎯 在 Xcode 中添加文件的步骤:"
echo ""
echo "1️⃣  打开项目:"
echo "   open WiseInvest.xcodeproj"
echo ""
echo "2️⃣  在 Xcode 中:"
echo "   - 右键点击左侧的 'WiseInvest' 文件夹"
echo "   - 选择 'Add Files to WiseInvest...'"
echo ""
echo "3️⃣  选择文件夹:"
echo "   - 导航到: $PROJECT_DIR/WiseInvest/"
echo "   - 选择这些文件夹(按住 Command 多选):"
echo "     • Core"
echo "     • Data"
echo "     • Domain"
echo "     • Presentation"
echo ""
echo "4️⃣  配置选项:"
echo "   ✅ 勾选 'Copy items if needed'"
echo "   ✅ 选择 'Create groups'"
echo "   ✅ 确保 'Add to targets: WiseInvest' 被勾选"
echo ""
echo "5️⃣  点击 'Add' 按钮"
echo ""
echo "6️⃣  验证:"
echo "   - 在项目导航器中应该看到完整的文件夹结构"
echo "   - 选择项目 → Build Phases → Compile Sources"
echo "   - 确认所有 .swift 文件都在列表中"
echo ""
echo "7️⃣  构建项目:"
echo "   - Clean: ⇧⌘K"
echo "   - Build: ⌘B"
echo ""

# Offer to open Xcode
echo "💡 提示:"
echo ""
read -p "是否现在打开 Xcode? (y/n) " -n 1 -r
echo ""

if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "🚀 正在打开 Xcode..."
    open "$PROJECT_FILE"
    echo ""
    echo "✅ Xcode 已打开,请按照上述步骤添加文件"
else
    echo "👍 好的,您可以稍后手动打开:"
    echo "   open $PROJECT_FILE"
fi

echo ""
echo "📖 详细说明请查看: SETUP_INSTRUCTIONS.md"
echo ""
