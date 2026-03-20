import SwiftUI

// MARK: - MessageBubble

struct MessageBubble: View {
    let message: Message
    var onRegenerate: (() -> Void)? = nil

    @State private var animationPhase = 0
    @State private var copied = false

    var body: some View {
        HStack(alignment: .top) {
            if message.role == .user {
                Spacer(minLength: 60)
            }

            VStack(alignment: message.role == .user ? .trailing : .leading, spacing: 4) {
                messageContent
                    .padding(12)
                    .background(backgroundColor)
                    .cornerRadius(16)

                if message.isStreaming {
                    StreamingIndicator(animationPhase: animationPhase)
                        .padding(.horizontal, 12)
                        .onAppear { startAnimation() }
                } else {
                    // 时间 + 操作按钮
                    HStack(spacing: 12) {
                        Text(formattedTime)
                            .font(.system(size: 12))
                            .foregroundColor(.textTertiary)

                        if message.role == .assistant {
                            messageActions
                        }
                    }
                    .padding(.horizontal, 12)
                }
            }

            if message.role == .assistant {
                Spacer(minLength: 60)
            }
        }
    }

    /// 复制 + 重新生成 按钮
    private var messageActions: some View {
        HStack(spacing: 4) {
            // 复制
            Button(action: copyContent) {
                Image(systemName: copied ? "checkmark" : "doc.on.doc")
                    .font(.system(size: 13))
                    .foregroundColor(copied ? Color(hex: "50C878") : .textSecondary)
                    .frame(width: 28, height: 28)
            }

            // 重新生成（仅最后一条消息）
            if let regenerate = onRegenerate {
                Button(action: regenerate) {
                    Image(systemName: "arrow.clockwise")
                        .font(.system(size: 13))
                        .foregroundColor(.textSecondary)
                        .frame(width: 28, height: 28)
                }
            }
        }
    }

    private func copyContent() {
        UIPasteboard.general.string = message.content
        withAnimation { copied = true }
        DispatchQueue.main.asyncAfter(deadline: .now() + 2) {
            withAnimation { copied = false }
        }
    }

    @ViewBuilder
    private var messageContent: some View {
        if message.role == .assistant {
            VStack(alignment: .leading, spacing: 8) {
                if !message.thinkingLines.isEmpty {
                    ThinkingBox(lines: message.thinkingLines)
                        .padding(10)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(Color.black.opacity(0.14))
                        .cornerRadius(8)
                }

                if !message.content.isEmpty {
                    MarkdownContentView(content: message.content)
                }
            }
        } else {
            Text(message.content)
                .font(.system(size: 16))
                .foregroundColor(.white)
        }
    }

    private var backgroundColor: Color {
        switch message.role {
        case .user:      return .userMessageBg
        case .assistant: return .assistantMessageBg
        case .system:    return .secondaryBackground
        }
    }

    private var formattedTime: String {
        let formatter = DateFormatter()
        formatter.timeStyle = .short
        return formatter.string(from: message.timestamp)
    }

    private func startAnimation() {
        Timer.scheduledTimer(withTimeInterval: 0.4, repeats: true) { timer in
            if !message.isStreaming {
                timer.invalidate()
                return
            }
            animationPhase = (animationPhase + 1) % 3
        }
    }
}

// MARK: - MarkdownContentView

/// 将 AI 回复按代码块分段渲染：代码块用专属样式，其余部分用系统 AttributedString 解析内联 Markdown
struct MarkdownContentView: View {
    let content: String

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            ForEach(Array(segments.enumerated()), id: \.offset) { _, segment in
                if segment.isCode {
                    CodeBlockView(code: segment.text, language: segment.language)
                } else if !segment.text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                    InlineMarkdownText(text: segment.text)
                }
            }
        }
    }

    /// 把 content 按 ``` 切成普通文本段和代码块段
    private var segments: [(text: String, isCode: Bool, language: String)] {
        var result: [(text: String, isCode: Bool, language: String)] = []
        let pattern = #"```([a-zA-Z]*)\n?([\s\S]*?)```"#
        guard let regex = try? NSRegularExpression(pattern: pattern) else {
            return [(text: content, isCode: false, language: "")]
        }

        let nsContent = content as NSString
        let range = NSRange(location: 0, length: nsContent.length)
        let matches = regex.matches(in: content, range: range)

        var cursor = 0
        for match in matches {
            let matchRange = match.range
            if matchRange.location > cursor {
                let before = nsContent.substring(with: NSRange(location: cursor, length: matchRange.location - cursor))
                result.append((text: before, isCode: false, language: ""))
            }
            let lang = match.range(at: 1).length > 0 ? nsContent.substring(with: match.range(at: 1)) : ""
            let code = match.range(at: 2).length > 0 ? nsContent.substring(with: match.range(at: 2)) : ""
            result.append((text: code, isCode: true, language: lang))
            cursor = matchRange.location + matchRange.length
        }
        if cursor < nsContent.length {
            let tail = nsContent.substring(from: cursor)
            result.append((text: tail, isCode: false, language: ""))
        }
        return result.isEmpty ? [(text: content, isCode: false, language: "")] : result
    }
}

// MARK: - InlineMarkdownText

/// 逐行渲染 Markdown，每行独立渲染到 VStack
struct InlineMarkdownText: View {
    let text: String

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            ForEach(Array(text.components(separatedBy: "\n").enumerated()), id: \.offset) { _, line in
                MarkdownLineView(line: line)
            }
        }
    }
}

// MARK: - MarkdownLineView

/// 单行 Markdown 渲染
/// 关键：使用 Text(LocalizedStringKey(string)) 而非 Text(string)
/// LocalizedStringKey 会触发 SwiftUI 内置 Markdown 解析（**粗体**、*斜体*、`code`、链接）
/// Text(string: String) 使用 StringProtocol 初始化器，不解析 Markdown
private struct MarkdownLineView: View {
    let line: String

    private var t: String { line.trimmingCharacters(in: .whitespaces) }

    var body: some View {
        Group {
            if t.isEmpty {
                // 空行 = 段落间距
                Color.clear.frame(height: 10)
            } else if t.hasPrefix("###### ") {
                Text(LocalizedStringKey(String(t.dropFirst(7))))
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundColor(.textSecondary)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.top, 6)
            } else if t.hasPrefix("##### ") {
                Text(LocalizedStringKey(String(t.dropFirst(6))))
                    .font(.system(size: 15, weight: .semibold))
                    .foregroundColor(.textSecondary)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.top, 6)
            } else if t.hasPrefix("#### ") {
                Text(LocalizedStringKey(String(t.dropFirst(5))))
                    .font(.system(size: 15, weight: .semibold))
                    .foregroundColor(.white)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.top, 8)
                    .padding(.bottom, 1)
            } else if t.hasPrefix("### ") {
                Text(LocalizedStringKey(String(t.dropFirst(4))))
                    .font(.system(size: 17, weight: .semibold))
                    .foregroundColor(.white)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.top, 10)
                    .padding(.bottom, 2)
            } else if t.hasPrefix("## ") {
                Text(LocalizedStringKey(String(t.dropFirst(3))))
                    .font(.system(size: 19, weight: .semibold))
                    .foregroundColor(.white)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.top, 12)
                    .padding(.bottom, 2)
            } else if t.hasPrefix("# ") {
                Text(LocalizedStringKey(String(t.dropFirst(2))))
                    .font(.system(size: 22, weight: .bold))
                    .foregroundColor(.white)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.top, 14)
                    .padding(.bottom, 4)
            } else if t.hasPrefix("- ") || t.hasPrefix("* ") {
                HStack(alignment: .top, spacing: 8) {
                    Text("•")
                        .font(.system(size: 14))
                        .foregroundColor(.textSecondary)
                        .frame(width: 12, alignment: .center)
                        .padding(.top, 2)
                    Text(LocalizedStringKey(String(t.dropFirst(2))))
                        .font(.system(size: 16))
                        .foregroundColor(.textPrimary)
                        .fixedSize(horizontal: false, vertical: true)
                }
                .padding(.vertical, 1)
            } else if t.range(of: #"^\d+\. "#, options: .regularExpression) != nil,
                      let dotIdx = t.firstIndex(of: ".") {
                // 数字列表：1. 2. 3.
                let num = String(t[t.startIndex..<dotIdx])
                let content = String(t[t.index(dotIdx, offsetBy: 2)...])
                HStack(alignment: .top, spacing: 8) {
                    Text("\(num).")
                        .font(.system(size: 15, weight: .medium))
                        .foregroundColor(.textSecondary)
                        .frame(width: 22, alignment: .trailing)
                    Text(LocalizedStringKey(content))
                        .font(.system(size: 16))
                        .foregroundColor(.textPrimary)
                        .fixedSize(horizontal: false, vertical: true)
                }
                .padding(.vertical, 1)
            } else if t.hasPrefix("> ") {
                HStack(alignment: .top, spacing: 10) {
                    Rectangle()
                        .fill(Color.accentBlue)
                        .frame(width: 3)
                        .cornerRadius(1.5)
                    Text(LocalizedStringKey(String(t.dropFirst(2))))
                        .font(.system(size: 15))
                        .foregroundColor(.textSecondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
                .padding(.vertical, 2)
            } else if t.hasPrefix("---") || t.hasPrefix("===") || t.hasPrefix("***") {
                // 水平分割线
                Divider()
                    .background(Color.textTertiary.opacity(0.5))
                    .padding(.vertical, 6)
            } else {
                // 普通段落：LocalizedStringKey 解析 **粗体** *斜体* `代码` [链接](url)
                Text(LocalizedStringKey(t))
                    .font(.system(size: 16))
                    .foregroundColor(.textPrimary)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.vertical, 1)
            }
        }
    }
}

// MARK: - CodeBlockView

/// 代码块：深色背景 + 等宽字体 + 可横向滚动 + 语言标签
struct CodeBlockView: View {
    let code: String
    let language: String
    @State private var copied = false

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            // 顶部工具栏
            HStack {
                if !language.isEmpty {
                    Text(language)
                        .font(.system(size: 12, weight: .medium))
                        .foregroundColor(Color(hex: "A0A0A0"))
                }
                Spacer()
                Button(action: copyCode) {
                    HStack(spacing: 4) {
                        Image(systemName: copied ? "checkmark" : "doc.on.doc")
                            .font(.system(size: 12))
                        Text(copied ? "已复制" : "复制")
                            .font(.system(size: 12))
                    }
                    .foregroundColor(copied ? Color(hex: "50C878") : Color(hex: "A0A0A0"))
                }
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
            .background(Color(hex: "161B2E"))

            Divider().background(Color(hex: "2C3E50"))

            // 代码内容
            ScrollView(.horizontal, showsIndicators: false) {
                Text(code.trimmingCharacters(in: .newlines))
                    .font(.system(size: 13, design: .monospaced))
                    .foregroundColor(Color(hex: "A8D8A8"))
                    .padding(12)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            .background(Color(hex: "0F1320"))
        }
        .cornerRadius(10)
        .overlay(
            RoundedRectangle(cornerRadius: 10)
                .stroke(Color(hex: "2C3E50"), lineWidth: 1)
        )
    }

    private func copyCode() {
        UIPasteboard.general.string = code
        withAnimation { copied = true }
        DispatchQueue.main.asyncAfter(deadline: .now() + 2) {
            withAnimation { copied = false }
        }
    }
}

// MARK: - ThinkingBox

private struct ThinkingBox: View {
    let lines: [String]

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            ForEach(Array(lines.prefix(4).enumerated()), id: \.offset) { _, line in
                Text(line)
                    .font(.system(size: 13))
                    .foregroundColor(.textSecondary)
                    .lineLimit(1)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

// MARK: - StreamingIndicator

struct StreamingIndicator: View {
    let animationPhase: Int

    var body: some View {
        HStack(spacing: 4) {
            ForEach(0..<3, id: \.self) { index in
                Circle()
                    .fill(Color.textSecondary)
                    .frame(width: 6, height: 6)
                    .opacity(streamingOpacity(for: index))
            }
        }
    }

    private func streamingOpacity(for index: Int) -> Double {
        let phase = (animationPhase + index) % 3
        return phase == 0 ? 1.0 : 0.3
    }
}
