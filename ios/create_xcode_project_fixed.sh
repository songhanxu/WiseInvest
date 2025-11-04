#!/bin/bash

# WiseInvest iOS Xcode Project Creation Script
# This script creates a properly configured Xcode project with correct deployment targets

set -e

PROJECT_DIR="/Users/songhanxu/WiseInvest/ios"
PROJECT_NAME="WiseInvest"
BUNDLE_ID="com.wiseinvest.app"
DEPLOYMENT_TARGET="15.0"

echo "🚀 Creating Xcode project for WiseInvest..."
echo "📁 Project directory: $PROJECT_DIR"
echo "📱 Deployment target: iOS $DEPLOYMENT_TARGET"

cd "$PROJECT_DIR"

# Check if Xcode is installed
if ! command -v xcodebuild &> /dev/null; then
    echo "❌ Error: Xcode is not installed or xcodebuild is not in PATH"
    exit 1
fi

# Remove existing project if it exists
if [ -d "$PROJECT_NAME.xcodeproj" ]; then
    echo "🗑️  Removing existing Xcode project..."
    rm -rf "$PROJECT_NAME.xcodeproj"
fi

# Create Xcode project using xcodebuild
echo "📦 Creating new Xcode project..."

# Create a temporary Package.swift for SPM-based project structure
cat > Package.swift << 'EOF'
// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "WiseInvest",
    platforms: [
        .iOS(.v15)
    ],
    products: [
        .library(
            name: "WiseInvest",
            targets: ["WiseInvest"])
    ],
    targets: [
        .target(
            name: "WiseInvest",
            path: "WiseInvest")
    ]
)
EOF

# Generate Xcode project from Package.swift
swift package generate-xcodeproj 2>/dev/null || {
    echo "⚠️  SPM method failed, using manual creation..."
    rm -f Package.swift
    
    # Create project manually using plutil and pbxproj
    mkdir -p "$PROJECT_NAME.xcodeproj"
    
    # Create project.pbxproj
    cat > "$PROJECT_NAME.xcodeproj/project.pbxproj" << 'PBXPROJ'
// !$*UTF8*$!
{
	archiveVersion = 1;
	classes = {
	};
	objectVersion = 56;
	objects = {

/* Begin PBXBuildFile section */
		WI001 /* WiseInvestApp.swift in Sources */ = {isa = PBXBuildFile; fileRef = WI002; };
		WI003 /* Assets.xcassets in Resources */ = {isa = PBXBuildFile; fileRef = WI004; };
/* End PBXBuildFile section */

/* Begin PBXFileReference section */
		WI000 /* WiseInvest.app */ = {isa = PBXFileReference; explicitFileType = wrapper.application; includeInIndex = 0; path = WiseInvest.app; sourceTree = BUILT_PRODUCTS_DIR; };
		WI002 /* WiseInvestApp.swift */ = {isa = PBXFileReference; lastKnownFileType = sourcecode.swift; path = WiseInvestApp.swift; sourceTree = "<group>"; };
		WI004 /* Assets.xcassets */ = {isa = PBXFileReference; lastKnownFileType = folder.assetcatalog; path = Assets.xcassets; sourceTree = "<group>"; };
/* End PBXFileReference section */

/* Begin PBXFrameworksBuildPhase section */
		WI100 /* Frameworks */ = {
			isa = PBXFrameworksBuildPhase;
			buildActionMask = 2147483647;
			files = (
			);
			runOnlyForDeploymentPostprocessing = 0;
		};
/* End PBXFrameworksBuildPhase section */

/* Begin PBXGroup section */
		WI200 = {
			isa = PBXGroup;
			children = (
				WI201 /* WiseInvest */,
				WI202 /* Products */,
			);
			sourceTree = "<group>";
		};
		WI201 /* WiseInvest */ = {
			isa = PBXGroup;
			children = (
				WI002 /* WiseInvestApp.swift */,
				WI004 /* Assets.xcassets */,
			);
			path = WiseInvest;
			sourceTree = "<group>";
		};
		WI202 /* Products */ = {
			isa = PBXGroup;
			children = (
				WI000 /* WiseInvest.app */,
			);
			name = Products;
			sourceTree = "<group>";
		};
/* End PBXGroup section */

/* Begin PBXNativeTarget section */
		WI300 /* WiseInvest */ = {
			isa = PBXNativeTarget;
			buildConfigurationList = WI301 /* Build configuration list for PBXNativeTarget "WiseInvest" */;
			buildPhases = (
				WI302 /* Sources */,
				WI100 /* Frameworks */,
				WI303 /* Resources */,
			);
			buildRules = (
			);
			dependencies = (
			);
			name = WiseInvest;
			productName = WiseInvest;
			productReference = WI000 /* WiseInvest.app */;
			productType = "com.apple.product-type.application";
		};
/* End PBXNativeTarget section */

/* Begin PBXProject section */
		WI400 /* Project object */ = {
			isa = PBXProject;
			attributes = {
				BuildIndependentTargetsInParallel = 1;
				LastSwiftUpdateCheck = 1500;
				LastUpgradeCheck = 1500;
				TargetAttributes = {
					WI300 = {
						CreatedOnToolsVersion = 15.0;
					};
				};
			};
			buildConfigurationList = WI401 /* Build configuration list for PBXProject "WiseInvest" */;
			compatibilityVersion = "Xcode 14.0";
			developmentRegion = en;
			hasScannedForEncodings = 0;
			knownRegions = (
				en,
				Base,
			);
			mainGroup = WI200;
			productRefGroup = WI202 /* Products */;
			projectDirPath = "";
			projectRoot = "";
			targets = (
				WI300 /* WiseInvest */,
			);
		};
/* End PBXProject section */

/* Begin PBXResourcesBuildPhase section */
		WI303 /* Resources */ = {
			isa = PBXResourcesBuildPhase;
			buildActionMask = 2147483647;
			files = (
				WI003 /* Assets.xcassets in Resources */,
			);
			runOnlyForDeploymentPostprocessing = 0;
		};
/* End PBXResourcesBuildPhase section */

/* Begin PBXSourcesBuildPhase section */
		WI302 /* Sources */ = {
			isa = PBXSourcesBuildPhase;
			buildActionMask = 2147483647;
			files = (
				WI001 /* WiseInvestApp.swift in Sources */,
			);
			runOnlyForDeploymentPostprocessing = 0;
		};
/* End PBXSourcesBuildPhase section */

/* Begin XCBuildConfiguration section */
		WI500 /* Debug */ = {
			isa = XCBuildConfiguration;
			buildSettings = {
				ALWAYS_SEARCH_USER_PATHS = NO;
				ASSETCATALOG_COMPILER_GENERATE_SWIFT_ASSET_SYMBOL_EXTENSIONS = YES;
				CLANG_ANALYZER_NONNULL = YES;
				CLANG_ANALYZER_NUMBER_OBJECT_CONVERSION = YES_AGGRESSIVE;
				CLANG_CXX_LANGUAGE_STANDARD = "gnu++20";
				CLANG_ENABLE_MODULES = YES;
				CLANG_ENABLE_OBJC_ARC = YES;
				CLANG_ENABLE_OBJC_WEAK = YES;
				CLANG_WARN_BLOCK_CAPTURE_AUTORELEASING = YES;
				CLANG_WARN_BOOL_CONVERSION = YES;
				CLANG_WARN_COMMA = YES;
				CLANG_WARN_CONSTANT_CONVERSION = YES;
				CLANG_WARN_DEPRECATED_OBJC_IMPLEMENTATIONS = YES;
				CLANG_WARN_DIRECT_OBJC_ISA_USAGE = YES_ERROR;
				CLANG_WARN_DOCUMENTATION_COMMENTS = YES;
				CLANG_WARN_EMPTY_BODY = YES;
				CLANG_WARN_ENUM_CONVERSION = YES;
				CLANG_WARN_INFINITE_RECURSION = YES;
				CLANG_WARN_INT_CONVERSION = YES;
				CLANG_WARN_NON_LITERAL_NULL_CONVERSION = YES;
				CLANG_WARN_OBJC_IMPLICIT_RETAIN_SELF = YES;
				CLANG_WARN_OBJC_LITERAL_CONVERSION = YES;
				CLANG_WARN_OBJC_ROOT_CLASS = YES_ERROR;
				CLANG_WARN_QUOTED_INCLUDE_IN_FRAMEWORK_HEADER = YES;
				CLANG_WARN_RANGE_LOOP_ANALYSIS = YES;
				CLANG_WARN_STRICT_PROTOTYPES = YES;
				CLANG_WARN_SUSPICIOUS_MOVE = YES;
				CLANG_WARN_UNGUARDED_AVAILABILITY = YES_AGGRESSIVE;
				CLANG_WARN_UNREACHABLE_CODE = YES;
				CLANG_WARN__DUPLICATE_METHOD_MATCH = YES;
				COPY_PHASE_STRIP = NO;
				DEBUG_INFORMATION_FORMAT = dwarf;
				ENABLE_STRICT_OBJC_MSGSEND = YES;
				ENABLE_TESTABILITY = YES;
				ENABLE_USER_SCRIPT_SANDBOXING = YES;
				GCC_C_LANGUAGE_STANDARD = gnu17;
				GCC_DYNAMIC_NO_PIC = NO;
				GCC_NO_COMMON_BLOCKS = YES;
				GCC_OPTIMIZATION_LEVEL = 0;
				GCC_PREPROCESSOR_DEFINITIONS = (
					"DEBUG=1",
					"$(inherited)",
				);
				GCC_WARN_64_TO_32_BIT_CONVERSION = YES;
				GCC_WARN_ABOUT_RETURN_TYPE = YES_ERROR;
				GCC_WARN_UNDECLARED_SELECTOR = YES;
				GCC_WARN_UNINITIALIZED_AUTOS = YES_AGGRESSIVE;
				GCC_WARN_UNUSED_FUNCTION = YES;
				GCC_WARN_UNUSED_VARIABLE = YES;
				IPHONEOS_DEPLOYMENT_TARGET = 15.0;
				LOCALIZATION_PREFERS_STRING_CATALOGS = YES;
				MTL_ENABLE_DEBUG_INFO = INCLUDE_SOURCE;
				MTL_FAST_MATH = YES;
				ONLY_ACTIVE_ARCH = YES;
				SDKROOT = iphoneos;
				SWIFT_ACTIVE_COMPILATION_CONDITIONS = "DEBUG $(inherited)";
				SWIFT_OPTIMIZATION_LEVEL = "-Onone";
			};
			name = Debug;
		};
		WI501 /* Release */ = {
			isa = XCBuildConfiguration;
			buildSettings = {
				ALWAYS_SEARCH_USER_PATHS = NO;
				ASSETCATALOG_COMPILER_GENERATE_SWIFT_ASSET_SYMBOL_EXTENSIONS = YES;
				CLANG_ANALYZER_NONNULL = YES;
				CLANG_ANALYZER_NUMBER_OBJECT_CONVERSION = YES_AGGRESSIVE;
				CLANG_CXX_LANGUAGE_STANDARD = "gnu++20";
				CLANG_ENABLE_MODULES = YES;
				CLANG_ENABLE_OBJC_ARC = YES;
				CLANG_ENABLE_OBJC_WEAK = YES;
				CLANG_WARN_BLOCK_CAPTURE_AUTORELEASING = YES;
				CLANG_WARN_BOOL_CONVERSION = YES;
				CLANG_WARN_COMMA = YES;
				CLANG_WARN_CONSTANT_CONVERSION = YES;
				CLANG_WARN_DEPRECATED_OBJC_IMPLEMENTATIONS = YES;
				CLANG_WARN_DIRECT_OBJC_ISA_USAGE = YES_ERROR;
				CLANG_WARN_DOCUMENTATION_COMMENTS = YES;
				CLANG_WARN_EMPTY_BODY = YES;
				CLANG_WARN_ENUM_CONVERSION = YES;
				CLANG_WARN_INFINITE_RECURSION = YES;
				CLANG_WARN_INT_CONVERSION = YES;
				CLANG_WARN_NON_LITERAL_NULL_CONVERSION = YES;
				CLANG_WARN_OBJC_IMPLICIT_RETAIN_SELF = YES;
				CLANG_WARN_OBJC_LITERAL_CONVERSION = YES;
				CLANG_WARN_OBJC_ROOT_CLASS = YES_ERROR;
				CLANG_WARN_QUOTED_INCLUDE_IN_FRAMEWORK_HEADER = YES;
				CLANG_WARN_RANGE_LOOP_ANALYSIS = YES;
				CLANG_WARN_STRICT_PROTOTYPES = YES;
				CLANG_WARN_SUSPICIOUS_MOVE = YES;
				CLANG_WARN_UNGUARDED_AVAILABILITY = YES_AGGRESSIVE;
				CLANG_WARN_UNREACHABLE_CODE = YES;
				CLANG_WARN__DUPLICATE_METHOD_MATCH = YES;
				COPY_PHASE_STRIP = NO;
				DEBUG_INFORMATION_FORMAT = "dwarf-with-dsym";
				ENABLE_NS_ASSERTIONS = NO;
				ENABLE_STRICT_OBJC_MSGSEND = YES;
				ENABLE_USER_SCRIPT_SANDBOXING = YES;
				GCC_C_LANGUAGE_STANDARD = gnu17;
				GCC_NO_COMMON_BLOCKS = YES;
				GCC_WARN_64_TO_32_BIT_CONVERSION = YES;
				GCC_WARN_ABOUT_RETURN_TYPE = YES_ERROR;
				GCC_WARN_UNDECLARED_SELECTOR = YES;
				GCC_WARN_UNINITIALIZED_AUTOS = YES_AGGRESSIVE;
				GCC_WARN_UNUSED_FUNCTION = YES;
				GCC_WARN_UNUSED_VARIABLE = YES;
				IPHONEOS_DEPLOYMENT_TARGET = 15.0;
				LOCALIZATION_PREFERS_STRING_CATALOGS = YES;
				MTL_ENABLE_DEBUG_INFO = NO;
				MTL_FAST_MATH = YES;
				SDKROOT = iphoneos;
				SWIFT_COMPILATION_MODE = wholemodule;
				VALIDATE_PRODUCT = YES;
			};
			name = Release;
		};
		WI502 /* Debug */ = {
			isa = XCBuildConfiguration;
			buildSettings = {
				ASSETCATALOG_COMPILER_APPICON_NAME = AppIcon;
				ASSETCATALOG_COMPILER_GLOBAL_ACCENT_COLOR_NAME = AccentColor;
				CODE_SIGN_STYLE = Automatic;
				CURRENT_PROJECT_VERSION = 1;
				DEVELOPMENT_ASSET_PATHS = "";
				ENABLE_PREVIEWS = YES;
				GENERATE_INFOPLIST_FILE = YES;
				INFOPLIST_KEY_UIApplicationSceneManifest_Generation = YES;
				INFOPLIST_KEY_UIApplicationSupportsIndirectInputEvents = YES;
				INFOPLIST_KEY_UILaunchScreen_Generation = YES;
				INFOPLIST_KEY_UISupportedInterfaceOrientations = "UIInterfaceOrientationPortrait UIInterfaceOrientationLandscapeLeft UIInterfaceOrientationLandscapeRight";
				INFOPLIST_KEY_UISupportedInterfaceOrientations_iPad = "UIInterfaceOrientationPortrait UIInterfaceOrientationPortraitUpsideDown UIInterfaceOrientationLandscapeLeft UIInterfaceOrientationLandscapeRight";
				IPHONEOS_DEPLOYMENT_TARGET = 15.0;
				LD_RUNPATH_SEARCH_PATHS = (
					"$(inherited)",
					"@executable_path/Frameworks",
				);
				MARKETING_VERSION = 1.0;
				PRODUCT_BUNDLE_IDENTIFIER = com.wiseinvest.app;
				PRODUCT_NAME = "$(TARGET_NAME)";
				SWIFT_EMIT_LOC_STRINGS = YES;
				SWIFT_VERSION = 5.0;
				TARGETED_DEVICE_FAMILY = "1,2";
			};
			name = Debug;
		};
		WI503 /* Release */ = {
			isa = XCBuildConfiguration;
			buildSettings = {
				ASSETCATALOG_COMPILER_APPICON_NAME = AppIcon;
				ASSETCATALOG_COMPILER_GLOBAL_ACCENT_COLOR_NAME = AccentColor;
				CODE_SIGN_STYLE = Automatic;
				CURRENT_PROJECT_VERSION = 1;
				DEVELOPMENT_ASSET_PATHS = "";
				ENABLE_PREVIEWS = YES;
				GENERATE_INFOPLIST_FILE = YES;
				INFOPLIST_KEY_UIApplicationSceneManifest_Generation = YES;
				INFOPLIST_KEY_UIApplicationSupportsIndirectInputEvents = YES;
				INFOPLIST_KEY_UILaunchScreen_Generation = YES;
				INFOPLIST_KEY_UISupportedInterfaceOrientations = "UIInterfaceOrientationPortrait UIInterfaceOrientationLandscapeLeft UIInterfaceOrientationLandscapeRight";
				INFOPLIST_KEY_UISupportedInterfaceOrientations_iPad = "UIInterfaceOrientationPortrait UIInterfaceOrientationPortraitUpsideDown UIInterfaceOrientationLandscapeLeft UIInterfaceOrientationLandscapeRight";
				IPHONEOS_DEPLOYMENT_TARGET = 15.0;
				LD_RUNPATH_SEARCH_PATHS = (
					"$(inherited)",
					"@executable_path/Frameworks",
				);
				MARKETING_VERSION = 1.0;
				PRODUCT_BUNDLE_IDENTIFIER = com.wiseinvest.app;
				PRODUCT_NAME = "$(TARGET_NAME)";
				SWIFT_EMIT_LOC_STRINGS = YES;
				SWIFT_VERSION = 5.0;
				TARGETED_DEVICE_FAMILY = "1,2";
			};
			name = Release;
		};
/* End XCBuildConfiguration section */

/* Begin XCConfigurationList section */
		WI301 /* Build configuration list for PBXNativeTarget "WiseInvest" */ = {
			isa = XCConfigurationList;
			buildConfigurations = (
				WI502 /* Debug */,
				WI503 /* Release */,
			);
			defaultConfigurationIsVisible = 0;
			defaultConfigurationName = Release;
		};
		WI401 /* Build configuration list for PBXProject "WiseInvest" */ = {
			isa = XCConfigurationList;
			buildConfigurations = (
				WI500 /* Debug */,
				WI501 /* Release */,
			);
			defaultConfigurationIsVisible = 0;
			defaultConfigurationName = Release;
		};
/* End XCConfigurationList section */
	};
	rootObject = WI400 /* Project object */;
}
PBXPROJ
}

# Clean up Package.swift if it exists
rm -f Package.swift

echo "✅ Xcode project structure created"

# Now we need to add all Swift files to the project
echo "📝 Adding Swift files to project..."

# Use a Python script to properly add files to the pbxproj
python3 << 'PYTHON_SCRIPT'
import os
import uuid
import re

project_dir = "/Users/songhanxu/WiseInvest/ios"
pbxproj_path = os.path.join(project_dir, "WiseInvest.xcodeproj", "project.pbxproj")

# Find all Swift files
swift_files = []
for root, dirs, files in os.walk(os.path.join(project_dir, "WiseInvest")):
    # Skip hidden directories and build directories
    dirs[:] = [d for d in dirs if not d.startswith('.') and d not in ['build', 'DerivedData']]
    for file in files:
        if file.endswith('.swift') and file != 'WiseInvestApp.swift':
            rel_path = os.path.relpath(os.path.join(root, file), os.path.join(project_dir, "WiseInvest"))
            swift_files.append((file, rel_path))

print(f"Found {len(swift_files)} Swift files to add")

# Read the current pbxproj
with open(pbxproj_path, 'r') as f:
    content = f.read()

# Generate UUIDs for new files
file_refs = []
build_files = []

for filename, relpath in swift_files:
    file_uuid = str(uuid.uuid4()).replace('-', '')[:24].upper()
    build_uuid = str(uuid.uuid4()).replace('-', '')[:24].upper()
    
    file_refs.append(f'\t\t{file_uuid} /* {filename} */ = {{isa = PBXFileReference; lastKnownFileType = sourcecode.swift; path = "{relpath}"; sourceTree = "<group>"; }};')
    build_files.append(f'\t\t{build_uuid} /* {filename} in Sources */ = {{isa = PBXBuildFile; fileRef = {file_uuid}; }};')
    
    # Add to Sources build phase
    sources_section = re.search(r'(WI302 /\* Sources \*/ = \{[^}]+files = \()', content)
    if sources_section:
        insert_pos = sources_section.end()
        content = content[:insert_pos] + f'\n\t\t\t\t{build_uuid} /* {filename} in Sources */,' + content[insert_pos:]

# Add file references
file_ref_section = re.search(r'(/\* End PBXFileReference section \*/)', content)
if file_ref_section:
    insert_pos = file_ref_section.start()
    content = content[:insert_pos] + '\n'.join(file_refs) + '\n' + content[insert_pos:]

# Add build files
build_file_section = re.search(r'(/\* End PBXBuildFile section \*/)', content)
if build_file_section:
    insert_pos = build_file_section.start()
    content = content[:insert_pos] + '\n'.join(build_files) + '\n' + content[insert_pos:]

# Write back
with open(pbxproj_path, 'w') as f:
    f.write(content)

print("✅ Added all Swift files to project")
PYTHON_SCRIPT

echo ""
echo "✅ Xcode project created successfully!"
echo ""
echo "📋 Project Details:"
echo "   - Project Name: $PROJECT_NAME"
echo "   - Bundle ID: $BUNDLE_ID"
echo "   - Deployment Target: iOS $DEPLOYMENT_TARGET"
echo "   - Location: $PROJECT_DIR/$PROJECT_NAME.xcodeproj"
echo ""
echo "🎯 Next Steps:"
echo "   1. Open the project: open $PROJECT_NAME.xcodeproj"
echo "   2. Select a development team in Signing & Capabilities"
echo "   3. Build and run (⌘R)"
echo ""
echo "💡 Tips:"
echo "   - The deployment target is set to iOS 15.0 (required for SwiftUI)"
echo "   - All Swift files have been added to the project"
echo "   - You may need to configure your OpenAI API key in APIClient.swift"
echo ""
