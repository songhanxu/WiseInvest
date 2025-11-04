#!/bin/bash

# WiseInvest iOS 项目创建脚本
# 自动创建 Xcode 项目并配置

set -e

# 颜色定义
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}"
echo "╔══════════════════════════════════════════════════════════╗"
echo "║                                                          ║"
echo "║        📱 WiseInvest iOS 项目创建脚本                    ║"
echo "║                                                          ║"
echo "╚══════════════════════════════════════════════════════════╝"
echo -e "${NC}"
echo ""

# 检查 Xcode 是否安装
if ! command -v xcodebuild &> /dev/null; then
    echo -e "${RED}❌ Xcode 未安装${NC}"
    echo -e "${YELLOW}请从 App Store 安装 Xcode${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Xcode 已安装 ($(xcodebuild -version | head -1))${NC}"
echo ""

# 当前目录
CURRENT_DIR=$(pwd)
PROJECT_NAME="WiseInvest"
BUNDLE_ID="com.wiseinvest.app"

echo -e "${YELLOW}📋 项目信息:${NC}"
echo -e "  项目名称: ${GREEN}$PROJECT_NAME${NC}"
echo -e "  Bundle ID: ${GREEN}$BUNDLE_ID${NC}"
echo -e "  位置: ${GREEN}$CURRENT_DIR${NC}"
echo ""

# 检查是否已存在项目
if [ -d "$PROJECT_NAME.xcodeproj" ]; then
    echo -e "${YELLOW}⚠️  项目已存在${NC}"
    read -p "是否删除并重新创建? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo -e "${YELLOW}删除现有项目...${NC}"
        rm -rf "$PROJECT_NAME.xcodeproj"
        rm -rf "$PROJECT_NAME.xcworkspace"
        rm -rf "Pods"
        rm -f "Podfile.lock"
    else
        echo -e "${YELLOW}取消操作${NC}"
        exit 0
    fi
fi

echo -e "${YELLOW}🎯 由于 Xcode 项目文件的复杂性，建议使用以下方法之一:${NC}"
echo ""
echo -e "${BLUE}方法 1: 使用 Xcode GUI (推荐)${NC}"
echo -e "  1. 打开 Xcode"
echo -e "  2. File → New → Project"
echo -e "  3. 选择 iOS → App"
echo -e "  4. 配置:"
echo -e "     - Product Name: ${GREEN}WiseInvest${NC}"
echo -e "     - Organization Identifier: ${GREEN}com.wiseinvest${NC}"
echo -e "     - Interface: ${GREEN}SwiftUI${NC}"
echo -e "     - Language: ${GREEN}Swift${NC}"
echo -e "  5. 保存到: ${GREEN}$CURRENT_DIR${NC}"
echo -e "  6. 删除自动生成的文件"
echo -e "  7. 添加现有的 WiseInvest 文件夹"
echo ""

echo -e "${BLUE}方法 2: 使用 Swift Package Manager${NC}"
echo -e "  创建一个 Swift Package 项目（更简单）"
echo ""

read -p "是否使用方法 2 创建 Swift Package 项目? (y/N): " -n 1 -r
echo

if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo -e "${YELLOW}创建 Swift Package 项目...${NC}"
    
    # 创建 Package.swift
    cat > Package.swift << 'EOF'
// swift-tools-version: 5.7
import PackageDescription

let package = Package(
    name: "WiseInvest",
    platforms: [
        .iOS(.v15)
    ],
    products: [
        .library(
            name: "WiseInvest",
            targets: ["WiseInvest"]),
    ],
    dependencies: [],
    targets: [
        .target(
            name: "WiseInvest",
            dependencies: [],
            path: "WiseInvest"),
    ]
)
EOF
    
    echo -e "${GREEN}✓ Package.swift 已创建${NC}"
    echo ""
    echo -e "${YELLOW}现在可以使用以下命令打开项目:${NC}"
    echo -e "  ${GREEN}open Package.swift${NC}"
    echo ""
    echo -e "${YELLOW}或者在 Xcode 中:${NC}"
    echo -e "  ${GREEN}File → Open → 选择 Package.swift${NC}"
    
else
    echo ""
    echo -e "${YELLOW}请按照方法 1 手动创建项目${NC}"
    echo ""
    echo -e "${BLUE}详细步骤请查看: ${GREEN}CREATE_XCODE_PROJECT.md${NC}"
    echo ""
    
    # 打开 Xcode
    read -p "是否现在打开 Xcode? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        open -a Xcode
        echo -e "${GREEN}✓ Xcode 已打开${NC}"
    fi
fi

echo ""
echo -e "${GREEN}🎉 完成！${NC}"
echo ""
