import SwiftUI

/// A text view that animates individual character changes with a vertical rolling effect.
///
/// When the displayed string updates:
/// - Characters that haven't changed stay in place (no animation).
/// - If the new numeric value is **larger**, changed characters roll **upward**
///   (old slides up and out, new slides up from below).
/// - If the new numeric value is **smaller**, changed characters roll **downward**
///   (old slides down and out, new slides down from above).
///
/// Usage — drop-in replacement for `Text(someString)`:
/// ```swift
/// RollingNumberText(
///     text: stock.priceText,
///     font: .system(size: 16, weight: .bold, design: .monospaced),
///     color: .white
/// )
/// ```
struct RollingNumberText: View {
    let text: String
    var font: Font = .body
    var color: Color = .primary
    /// Animation duration per character flip
    var animationDuration: Double = 0.35
    /// Minimum scale factor (mirrors Text.minimumScaleFactor)
    var minimumScaleFactor: CGFloat = 1.0

    /// Internal state that drives the per-character animation.
    @State private var displayedCharacters: [AnimatedCharacter] = []
    /// The previously rendered text — used to detect direction of change.
    @State private var previousText: String = ""

    var body: some View {
        HStack(spacing: 0) {
            ForEach(displayedCharacters) { char in
                SingleCharacterView(
                    character: char.character,
                    previousCharacter: char.previousCharacter,
                    direction: char.direction,
                    font: font,
                    color: color,
                    animationDuration: animationDuration,
                    minimumScaleFactor: minimumScaleFactor
                )
                .id("\(char.position)-\(char.character)")
            }
        }
        .onAppear {
            // Initialize without animation
            initializeCharacters(from: text)
        }
        .onChange(of: text) { newText in
            updateCharacters(to: newText)
        }
    }

    // MARK: - Logic

    private func initializeCharacters(from str: String) {
        previousText = str
        displayedCharacters = str.enumerated().map { index, char in
            AnimatedCharacter(
                position: index,
                character: String(char),
                previousCharacter: String(char),
                direction: .none
            )
        }
    }

    private func updateCharacters(to newText: String) {
        let oldText = previousText
        previousText = newText

        // Determine overall scroll direction by comparing numeric values
        let direction = Self.resolveDirection(old: oldText, new: newText)

        // Align old and new strings for per-character diff.
        // Pad the shorter one on the LEFT so digits line up from the right
        // (e.g. "12.34" → " 12.34" vs "112.34").
        let maxLen = max(oldText.count, newText.count)
        let paddedOld = oldText.leftPadded(toLength: maxLen)
        let paddedNew = newText.leftPadded(toLength: maxLen)

        var newChars: [AnimatedCharacter] = []
        for (index, (oldChar, newChar)) in zip(paddedOld, paddedNew).enumerated() {
            let oldStr = String(oldChar)
            let newStr = String(newChar)
            let changed = oldStr != newStr
            newChars.append(AnimatedCharacter(
                position: index,
                character: newStr,
                previousCharacter: oldStr,
                direction: changed ? direction : .none
            ))
        }

        displayedCharacters = newChars
    }

    /// Compare two formatted number strings and return the scroll direction.
    static func resolveDirection(old: String, new: String) -> ScrollDirection {
        // Extract the numeric part (strip %, +, spaces, etc.)
        let oldVal = numericValue(of: old)
        let newVal = numericValue(of: new)

        if let o = oldVal, let n = newVal {
            if n > o { return .up }
            if n < o { return .down }
            return .none
        }
        // Fallback: lexicographic comparison
        if new > old { return .up }
        if new < old { return .down }
        return .none
    }

    /// Attempt to parse a numeric value from a formatted string like "+1.23%", "3,245.67", etc.
    private static func numericValue(of str: String) -> Double? {
        let cleaned = str
            .replacingOccurrences(of: "%", with: "")
            .replacingOccurrences(of: ",", with: "")
            .replacingOccurrences(of: " ", with: "")
            .replacingOccurrences(of: "亿", with: "")
            .replacingOccurrences(of: "+", with: "")
        return Double(cleaned)
    }
}

// MARK: - Direction

enum ScrollDirection {
    case up    // value increased → old rolls up, new enters from bottom
    case down  // value decreased → old rolls down, new enters from top
    case none  // no change
}

// MARK: - Animated Character Model

struct AnimatedCharacter: Identifiable {
    let id = UUID()
    let position: Int
    let character: String
    let previousCharacter: String
    let direction: ScrollDirection
}

// MARK: - Single Character Animation View

private struct SingleCharacterView: View {
    let character: String
    let previousCharacter: String
    let direction: ScrollDirection
    let font: Font
    let color: Color
    let animationDuration: Double
    let minimumScaleFactor: CGFloat

    @State private var animationProgress: CGFloat = 0

    var body: some View {
        // We measure the character size by rendering an invisible reference character
        // and overlay the animated content on top.
        ZStack {
            // Invisible sizer — ensures consistent cell width for monospaced fonts.
            Text("0")
                .font(font)
                .opacity(0)

            if direction == .none {
                // No change — static text
                Text(character)
                    .font(font)
                    .foregroundColor(color)
                    .minimumScaleFactor(minimumScaleFactor)
            } else {
                // Animated transition
                GeometryReader { geo in
                    let h = geo.size.height
                    ZStack {
                        // Old character sliding out
                        Text(previousCharacter)
                            .font(font)
                            .foregroundColor(color)
                            .minimumScaleFactor(minimumScaleFactor)
                            .offset(y: oldCharacterOffset(height: h))
                            .opacity(1 - animationProgress)

                        // New character sliding in
                        Text(character)
                            .font(font)
                            .foregroundColor(color)
                            .minimumScaleFactor(minimumScaleFactor)
                            .offset(y: newCharacterOffset(height: h))
                            .opacity(animationProgress)
                    }
                    .frame(width: geo.size.width, height: geo.size.height)
                    .clipped()
                }
                .onAppear {
                    animationProgress = 0
                    withAnimation(.easeInOut(duration: animationDuration)) {
                        animationProgress = 1
                    }
                }
            }
        }
        // Ensure consistent width: use fixedSize so each character cell is exactly one char wide
        .fixedSize(horizontal: true, vertical: false)
    }

    // MARK: - Offset Calculations

    /// Old character: slides out in the direction of change.
    /// Up → old moves up (negative Y); Down → old moves down (positive Y).
    private func oldCharacterOffset(height: CGFloat) -> CGFloat {
        switch direction {
        case .up:   return -height * 0.6 * animationProgress
        case .down: return  height * 0.6 * animationProgress
        case .none: return 0
        }
    }

    /// New character: enters from the opposite side.
    /// Up → new enters from below; Down → new enters from above.
    private func newCharacterOffset(height: CGFloat) -> CGFloat {
        switch direction {
        case .up:   return  height * 0.6 * (1 - animationProgress)
        case .down: return -height * 0.6 * (1 - animationProgress)
        case .none: return 0
        }
    }
}

// MARK: - String Helper

private extension String {
    /// Left-pad with spaces to reach the desired length.
    func leftPadded(toLength length: Int) -> String {
        if count >= length { return self }
        return String(repeating: " ", count: length - count) + self
    }
}
