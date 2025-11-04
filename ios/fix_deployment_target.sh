#!/bin/bash

# WiseInvest - Fix Deployment Target Script
# This script helps fix the iOS deployment target issue

set -e

PROJECT_DIR="/Users/songhanxu/WiseInvest/ios"
PROJECT_FILE="$PROJECT_DIR/WiseInvest.xcodeproj/project.pbxproj"

echo "🔧 WiseInvest Deployment Target Fix"
echo "===================================="
echo ""

# Check if Xcode project exists
if [ ! -f "$PROJECT_FILE" ]; then
    echo "❌ Error: Xcode project not found at $PROJECT_FILE"
    echo ""
    echo "📖 Please follow these steps to create the project:"
    echo ""
    echo "1. Open Xcode"
    echo "2. File → New → Project"
    echo "3. Choose iOS → App"
    echo "4. Product Name: WiseInvest"
    echo "5. Interface: SwiftUI"
    echo "6. Language: Swift"
    echo "7. Save to: $PROJECT_DIR"
    echo ""
    echo "Then run this script again, or see XCODE_SETUP_GUIDE.md for detailed instructions."
    exit 1
fi

echo "✅ Found Xcode project"
echo ""

# Backup the project file
echo "📦 Creating backup..."
cp "$PROJECT_FILE" "$PROJECT_FILE.backup.$(date +%Y%m%d_%H%M%S)"
echo "✅ Backup created"
echo ""

# Fix deployment target using sed
echo "🔧 Updating deployment target to iOS 15.0..."

# Replace IPHONEOS_DEPLOYMENT_TARGET
if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS sed
    sed -i '' 's/IPHONEOS_DEPLOYMENT_TARGET = [0-9.]*;/IPHONEOS_DEPLOYMENT_TARGET = 15.0;/g' "$PROJECT_FILE"
else
    # Linux sed
    sed -i 's/IPHONEOS_DEPLOYMENT_TARGET = [0-9.]*;/IPHONEOS_DEPLOYMENT_TARGET = 15.0;/g' "$PROJECT_FILE"
fi

echo "✅ Deployment target updated"
echo ""

# Verify the change
if grep -q "IPHONEOS_DEPLOYMENT_TARGET = 15.0;" "$PROJECT_FILE"; then
    echo "✅ Verification successful - deployment target is now iOS 15.0"
else
    echo "⚠️  Warning: Could not verify the change automatically"
    echo "   Please check manually in Xcode:"
    echo "   Project Settings → General → Minimum Deployments"
fi

echo ""
echo "🎉 Fix completed!"
echo ""
echo "📋 Next steps:"
echo "   1. Close Xcode if it's open"
echo "   2. Open the project: open WiseInvest.xcodeproj"
echo "   3. Verify deployment target in Project Settings → General"
echo "   4. Clean build folder: Product → Clean Build Folder (⇧⌘K)"
echo "   5. Build: Product → Build (⌘B)"
echo ""
echo "💡 If you still see errors:"
echo "   - Make sure all Swift files are added to the project"
echo "   - Check that WiseInvestApp.swift is in the project"
echo "   - See XCODE_SETUP_GUIDE.md for detailed setup instructions"
echo ""
