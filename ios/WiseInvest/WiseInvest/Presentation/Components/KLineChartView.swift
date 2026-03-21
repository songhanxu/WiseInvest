import SwiftUI
import QuartzCore

/// Mini sparkline chart for market index cards
struct SparklineView: View {
    let data: [Double]
    let isUp: Bool

    var body: some View {
        GeometryReader { geometry in
            let width = geometry.size.width
            let height = geometry.size.height
            guard data.count > 1 else { return AnyView(EmptyView()) }
            let minVal = data.min() ?? 0
            let maxVal = data.max() ?? 1
            let range = maxVal - minVal
            let step = width / CGFloat(data.count - 1)

            return AnyView(
                Path { path in
                    for (index, value) in data.enumerated() {
                        let x = step * CGFloat(index)
                        let y = height - (range > 0 ? CGFloat((value - minVal) / range) * height : height / 2)
                        if index == 0 {
                            path.move(to: CGPoint(x: x, y: y))
                        } else {
                            path.addLine(to: CGPoint(x: x, y: y))
                        }
                    }
                }
                .stroke(isUp ? Color.accentGreen : Color(hex: "E53935"), lineWidth: 1.5)
            )
        }
    }
}

// MARK: - Interactive K-Line Chart (Candlestick)

/// Manages scroll, zoom, and crosshair state for the K-line chart.
class KLineChartState: ObservableObject {
    /// Number of visible candles (initial = 40 for a clean view)
    @Published var visibleCount: Int = 40
    /// Offset from the rightmost candle (0 = showing latest data)
    @Published var scrollOffset: Int = 0
    /// Crosshair index (relative to the full data array). nil = no crosshair
    @Published var crosshairIndex: Int? = nil
    /// Whether user is currently touching for crosshair
    @Published var isTouching: Bool = false

    /// Vertical scale factor (1.0 = auto-fit, >1 = zoomed in vertically)
    @Published var verticalScale: CGFloat = 1.0
    /// Vertical offset for panning after vertical zoom (in price-space ratio)
    @Published var verticalOffset: CGFloat = 0.0

    /// Dynamic right margin in pixels. Starts at `initialRightMargin` (30px) so
    /// the newest candle is offset from the right edge. When the user scrolls
    /// right (drags right) while at scrollOffset == 0, this margin shrinks toward
    /// 0 instead of changing scrollOffset. When scrolling back left and
    /// scrollOffset is 0, this margin grows back up to its initial value before
    /// scrollOffset starts increasing.
    @Published var pixelRightMargin: CGFloat = 30

    // Gesture accumulators
    var dragAccumulator: CGFloat = 0

    // Pinch gesture state
    var pinchBaseVisibleCount: Int = 40
    var isPinching: Bool = false

    // Vertical drag state (price axis single-finger drag)
    var verticalDragBaseScale: CGFloat = 1.0
    var isVerticalDragging: Bool = false

    let minVisibleCount = 15
    let maxVisibleCount = 300

    let minVerticalScale: CGFloat = 0.5
    let maxVerticalScale: CGFloat = 5.0

    /// Whether we've requested more historical data (debounce flag)
    var hasRequestedMore: Bool = false

    /// Selected period (set by parent to determine time label format)
    var currentPeriod: String = "1d"

    // MARK: - Momentum / Inertia Scrolling

    /// Current inertia velocity in pixels/second (positive = scrolling right / showing newer)
    var momentumVelocity: CGFloat = 0
    /// DisplayLink that drives per-frame deceleration updates
    private var displayLink: CADisplayLink?
    /// Timestamp of the last DisplayLink callback (for computing dt)
    private var lastDisplayLinkTimestamp: CFTimeInterval = 0
    /// Deceleration rate per second (velocity is multiplied by this factor each second).
    /// 0.96 feels natural on iOS — similar to UIScrollView's normal deceleration.
    private let decelerationRate: CGFloat = 0.96
    /// Minimum velocity (px/s) below which the animation stops.
    private let minimumVelocity: CGFloat = 15
    /// Cached layout values set before starting momentum so the tick handler can use them.
    private var momentumChartWidth: CGFloat = 0
    private var momentumDataCount: Int = 0
    private var momentumInitialRightMargin: CGFloat = 30
    /// Callbacks set before starting momentum
    var momentumOnScrollChanged: (() -> Void)?
    var momentumOnLoadMoreCheck: (() -> Void)?

    /// Begin inertia animation with the given initial velocity (pixels/second).
    func startMomentum(velocity: CGFloat, chartWidth: CGFloat, dataCount: Int,
                       initialRightMargin: CGFloat,
                       onScrollChanged: @escaping () -> Void,
                       onLoadMoreCheck: @escaping () -> Void) {
        // Ignore tiny velocities
        guard abs(velocity) > minimumVelocity else { return }

        stopMomentum()

        momentumVelocity = velocity
        momentumChartWidth = chartWidth
        momentumDataCount = dataCount
        momentumInitialRightMargin = initialRightMargin
        momentumOnScrollChanged = onScrollChanged
        momentumOnLoadMoreCheck = onLoadMoreCheck
        lastDisplayLinkTimestamp = 0

        let link = CADisplayLink(target: self, selector: #selector(momentumTick))
        link.add(to: .main, forMode: .common)
        displayLink = link
    }

    /// Stop any running inertia animation immediately.
    func stopMomentum() {
        displayLink?.invalidate()
        displayLink = nil
        momentumVelocity = 0
        lastDisplayLinkTimestamp = 0
    }

    @objc private func momentumTick(_ link: CADisplayLink) {
        let now = link.timestamp
        if lastDisplayLinkTimestamp == 0 {
            // First frame — just record the timestamp, don't move yet
            lastDisplayLinkTimestamp = now
            return
        }

        let dt = CGFloat(now - lastDisplayLinkTimestamp)
        lastDisplayLinkTimestamp = now

        // Clamp dt to avoid huge jumps if the app was backgrounded
        let clampedDt = min(dt, 0.05)

        // Apply deceleration: v = v * rate^dt
        // For dt ≈ 1/60 this gives a smooth per-frame decay
        let decay = pow(decelerationRate, clampedDt * 60) // 60fps-normalized
        momentumVelocity *= decay

        // Stop if velocity is negligible
        if abs(momentumVelocity) < minimumVelocity {
            stopMomentum()
            return
        }

        // Compute pixel delta for this frame
        let pixelDelta = momentumVelocity * clampedDt

        // Reuse the same two-layer scrolling logic as handlePan
        let didScroll = applyPixelDelta(pixelDelta,
                                        chartWidth: momentumChartWidth,
                                        dataCount: momentumDataCount,
                                        initialRightMargin: momentumInitialRightMargin)
        if didScroll {
            momentumOnScrollChanged?()
            momentumOnLoadMoreCheck?()
        }
    }

    /// Apply a pixel-level delta to the two-layer scroll model.
    /// Shared by both handlePan (direct touch) and momentum (inertia).
    /// Returns `true` if scrollOffset actually changed (candle-level movement).
    @discardableResult
    func applyPixelDelta(_ delta: CGFloat, chartWidth: CGFloat, dataCount: Int,
                         initialRightMargin: CGFloat) -> Bool {
        let candleWidth = chartWidth / CGFloat(visibleCount)
        guard candleWidth > 0 else { return false }

        dragAccumulator += delta

        // --- Layer 1: pixel-level right margin ---
        if scrollOffset == 0 {
            let currentMargin = pixelRightMargin
            let accumulator = dragAccumulator

            if accumulator > 0 && currentMargin > 0 {
                // Scrolling right — shrink the margin
                let consume = min(accumulator, currentMargin)
                dragAccumulator -= consume
                pixelRightMargin = currentMargin - consume
            } else if accumulator < 0 && currentMargin < initialRightMargin {
                // Scrolling left — grow the margin back
                let deficit = initialRightMargin - currentMargin
                let consume = min(-accumulator, deficit)
                dragAccumulator += consume
                pixelRightMargin = currentMargin + consume
            }
        }

        // --- Layer 2: candle-level scrolling ---
        let candleDelta = Int(dragAccumulator / candleWidth)
        if candleDelta != 0 {
            dragAccumulator -= CGFloat(candleDelta) * candleWidth
            let newOffset = scrollOffset + candleDelta
            let maxOffset = max(0, dataCount - visibleCount)
            let clampedOffset = min(max(0, newOffset), maxOffset)

            // If we hit a boundary, stop the momentum (no rubber-banding)
            if clampedOffset != newOffset {
                stopMomentum()
                dragAccumulator = 0
            }

            scrollOffset = clampedOffset
            return true
        }
        return false
    }

    func reset() {
        stopMomentum()
        visibleCount = 40
        scrollOffset = 0
        crosshairIndex = nil
        isTouching = false
        dragAccumulator = 0
        pinchBaseVisibleCount = 40
        isPinching = false
        verticalScale = 1.0
        verticalOffset = 0.0
        verticalDragBaseScale = 1.0
        isVerticalDragging = false
        hasRequestedMore = false
        pixelRightMargin = 30
    }

    deinit {
        stopMomentum()
    }
}

// MARK: - UIKit Gesture Handler (bridges UIKit gestures to SwiftUI)

/// A transparent UIView overlay that captures pan, pinch, two-finger-pan, and long-press gestures.
/// This bypasses SwiftUI's gesture system for precise control over what goes to ScrollView.
///
/// Gesture routing:
/// - Single-finger horizontal pan → captured (K-line horizontal scroll)
/// - Single-finger vertical pan → NOT captured → passes to ScrollView (page scroll) ← KEY!
/// - Two-finger pinch → captured (horizontal zoom), temporarily disables ScrollView
/// - Two-finger pan (any direction) → captured (vertical zoom), temporarily disables ScrollView
/// - Long press → captured (crosshair)
///
/// **ScrollView restoration strategy**:
/// The core problem is that `UIScrollView.isScrollEnabled = false` can get "stuck" if the
/// gesture state machine doesn't perfectly balance disable/enable calls. To fix this robustly:
/// 1. `wasScrollEnabled` is only captured on the FIRST disable call (not overwritten on repeated calls)
/// 2. A `scrollDisableCount` tracks nested disable calls; enable only fires when count reaches 0
/// 3. `touchesEnded`/`touchesCancelled` acts as a safety net that force-restores when all fingers lift
/// 4. A deferred restoration timer fires 0.15s after the last enable, ensuring the scroll state is
///    always restored even if the gesture system misbehaves
class KLineGestureView: UIView, UIGestureRecognizerDelegate {
    var onPan: ((UIPanGestureRecognizer) -> Void)?
    var onPinch: ((UIPinchGestureRecognizer) -> Void)?
    var onTwoFingerPan: ((UIPanGestureRecognizer) -> Void)?
    var onLongPress: ((UILongPressGestureRecognizer) -> Void)?

    /// Reference to the parent ScrollView (found lazily) to disable scrolling during multi-touch
    private weak var parentScrollView: UIScrollView?
    /// The original scroll-enabled state before we first disabled it.
    /// Only captured on the first `disableParentScroll()` call in a gesture sequence.
    private var savedScrollEnabled: Bool = true
    /// The original bounces state.
    private var savedBounces: Bool = true
    /// How many times `disableParentScroll()` has been called without a matching `enableParentScroll()`.
    /// Prevents the second disable call from overwriting `savedScrollEnabled` with `false`.
    private var scrollDisableCount: Int = 0
    /// Track whether long press is active to prevent pan from starting during crosshair
    private var isLongPressActive: Bool = false
    /// Track whether any two-finger gesture is active
    private var isTwoFingerActive: Bool = false
    /// Deferred scroll-restore timer. After all gestures end, we schedule a short delay
    /// to force-restore scroll state as a final safety net.
    private var deferredRestoreTimer: Timer?

    // Single-finger horizontal pan (K-line scroll)
    private lazy var panGesture: UIPanGestureRecognizer = {
        let g = UIPanGestureRecognizer(target: self, action: #selector(handlePan(_:)))
        g.delegate = self
        g.minimumNumberOfTouches = 1
        g.maximumNumberOfTouches = 1
        return g
    }()

    // Two-finger pinch (horizontal zoom)
    private lazy var pinchGesture: UIPinchGestureRecognizer = {
        let g = UIPinchGestureRecognizer(target: self, action: #selector(handlePinch(_:)))
        g.delegate = self
        return g
    }()

    // Two-finger pan (vertical zoom — captures two-finger vertical/diagonal swipe)
    private lazy var twoFingerPanGesture: UIPanGestureRecognizer = {
        let g = UIPanGestureRecognizer(target: self, action: #selector(handleTwoFingerPan(_:)))
        g.delegate = self
        g.minimumNumberOfTouches = 2
        g.maximumNumberOfTouches = 2
        return g
    }()

    private lazy var longPressGesture: UILongPressGestureRecognizer = {
        let g = UILongPressGestureRecognizer(target: self, action: #selector(handleLongPress(_:)))
        g.minimumPressDuration = 0.25
        g.delegate = self
        return g
    }()

    override init(frame: CGRect) {
        super.init(frame: frame)
        setupGestures()
    }

    required init?(coder: NSCoder) {
        super.init(coder: coder)
        setupGestures()
    }

    private func setupGestures() {
        addGestureRecognizer(panGesture)
        addGestureRecognizer(pinchGesture)
        addGestureRecognizer(twoFingerPanGesture)
        addGestureRecognizer(longPressGesture)
        // Enable multi-touch so we can track all fingers
        isMultipleTouchEnabled = true
    }

    /// Safety net: when ALL fingers leave the view, force-reset the scroll state.
    /// This handles edge cases where gesture recognizer callbacks don't perfectly balance.
    override func touchesEnded(_ touches: Set<UITouch>, with event: UIEvent?) {
        super.touchesEnded(touches, with: event)
        forceResetIfNoTouches(event: event)
    }

    override func touchesCancelled(_ touches: Set<UITouch>, with event: UIEvent?) {
        super.touchesCancelled(touches, with: event)
        forceResetIfNoTouches(event: event)
    }

    private func forceResetIfNoTouches(event: UIEvent?) {
        // Count remaining active touches on this view
        let remaining = event?.touches(for: self)?.filter {
            $0.phase == .began || $0.phase == .moved || $0.phase == .stationary
        }.count ?? 0

        if remaining == 0 {
            // ALL fingers have lifted — unconditionally restore scroll state
            forceRestoreScroll()
        }
    }

    /// Unconditionally restore the parent ScrollView to its original state and reset all internal flags.
    /// This is the nuclear option — called when we're certain all touches have ended.
    private func forceRestoreScroll() {
        deferredRestoreTimer?.invalidate()
        deferredRestoreTimer = nil
        scrollDisableCount = 0
        isTwoFingerActive = false
        isLongPressActive = false

        if let sv = parentScrollView {
            sv.isScrollEnabled = savedScrollEnabled
            sv.bounces = savedBounces
        }
    }

    /// Walk up the view hierarchy to find the enclosing UIScrollView
    private func findParentScrollView() -> UIScrollView? {
        var view: UIView? = self.superview
        while let v = view {
            if let scrollView = v as? UIScrollView {
                return scrollView
            }
            view = v.superview
        }
        return nil
    }

    /// Disable parent ScrollView scrolling.
    /// Only saves the original state on the FIRST call (when `scrollDisableCount` goes from 0→1).
    /// Subsequent calls just increment the count without overwriting `savedScrollEnabled`.
    private func disableParentScroll() {
        // Cancel any pending deferred restore since a new gesture is starting
        deferredRestoreTimer?.invalidate()
        deferredRestoreTimer = nil

        if parentScrollView == nil {
            parentScrollView = findParentScrollView()
        }
        if scrollDisableCount == 0 {
            // First disable — capture the original state
            if let sv = parentScrollView {
                savedScrollEnabled = sv.isScrollEnabled
                savedBounces = sv.bounces
            }
        }
        scrollDisableCount += 1

        if let sv = parentScrollView {
            sv.isScrollEnabled = false
            sv.bounces = false
        }
    }

    /// Re-enable parent ScrollView scrolling.
    /// Decrements the disable count; only actually restores scroll when count reaches 0.
    /// Also schedules a deferred safety restore in case the count never reaches 0.
    private func enableParentScroll() {
        scrollDisableCount = max(0, scrollDisableCount - 1)

        if scrollDisableCount == 0 {
            if let sv = parentScrollView {
                sv.isScrollEnabled = savedScrollEnabled
                sv.bounces = savedBounces
            }
        }

        // Schedule a deferred safety restore. If for any reason the count doesn't reach 0
        // (gesture system bug), this timer will force-restore after a short delay.
        // It is cancelled if a new disableParentScroll() call arrives before it fires.
        scheduleDeferredRestore()
    }

    /// Schedule a deferred force-restore of scroll state, firing 0.15s after the last
    /// enableParentScroll() call. This is a final safety net that catches any edge case
    /// where the gesture lifecycle doesn't perfectly balance disable/enable calls.
    private func scheduleDeferredRestore() {
        deferredRestoreTimer?.invalidate()
        deferredRestoreTimer = Timer.scheduledTimer(withTimeInterval: 0.15, repeats: false) { [weak self] _ in
            guard let self = self else { return }
            // Only force-restore if no gesture is actively recognized right now
            let pinchActive = self.pinchGesture.state == .began || self.pinchGesture.state == .changed
            let twoFingerPanActive = self.twoFingerPanGesture.state == .began || self.twoFingerPanGesture.state == .changed
            let longPressActive = self.longPressGesture.state == .began || self.longPressGesture.state == .changed

            if !pinchActive && !twoFingerPanActive && !longPressActive {
                self.forceRestoreScroll()
            }
        }
    }

    @objc private func handlePan(_ gesture: UIPanGestureRecognizer) {
        // If long press or two-finger gesture is active, cancel single-finger pan
        if isLongPressActive || isTwoFingerActive {
            gesture.isEnabled = false
            gesture.isEnabled = true
            return
        }
        onPan?(gesture)
    }

    @objc private func handlePinch(_ gesture: UIPinchGestureRecognizer) {
        switch gesture.state {
        case .began:
            isTwoFingerActive = true
            disableParentScroll()
        case .changed:
            break
        case .ended, .cancelled, .failed:
            enableParentScroll()
            // Only clear the two-finger flag if the other two-finger gesture is also done
            if twoFingerPanGesture.state != .began && twoFingerPanGesture.state != .changed {
                isTwoFingerActive = false
            }
        default:
            break
        }
        onPinch?(gesture)
    }

    @objc private func handleTwoFingerPan(_ gesture: UIPanGestureRecognizer) {
        switch gesture.state {
        case .began:
            isTwoFingerActive = true
            disableParentScroll()
            // Reset translation to zero to prevent accumulated offset from causing jitter
            gesture.setTranslation(.zero, in: self)
        case .changed:
            break
        case .ended, .cancelled, .failed:
            enableParentScroll()
            // Only clear the two-finger flag if the other two-finger gesture is also done
            if pinchGesture.state != .began && pinchGesture.state != .changed {
                isTwoFingerActive = false
            }
        default:
            break
        }
        onTwoFingerPan?(gesture)
    }

    @objc private func handleLongPress(_ gesture: UILongPressGestureRecognizer) {
        switch gesture.state {
        case .began:
            isLongPressActive = true
            disableParentScroll()
        case .ended, .cancelled, .failed:
            isLongPressActive = false
            enableParentScroll()
        default:
            break
        }
        onLongPress?(gesture)
    }

    // MARK: - UIGestureRecognizerDelegate

    func gestureRecognizer(_ gestureRecognizer: UIGestureRecognizer,
                           shouldRecognizeSimultaneouslyWith otherGestureRecognizer: UIGestureRecognizer) -> Bool {
        // Allow pinch + two-finger-pan simultaneously (both are two-finger gestures)
        if (gestureRecognizer is UIPinchGestureRecognizer && otherGestureRecognizer == twoFingerPanGesture) ||
           (gestureRecognizer == twoFingerPanGesture && otherGestureRecognizer is UIPinchGestureRecognizer) {
            return true
        }
        return false
    }

    /// Only claim horizontal single-finger pans; let vertical touches pass through to ScrollView.
    override func gestureRecognizerShouldBegin(_ gestureRecognizer: UIGestureRecognizer) -> Bool {
        if gestureRecognizer == panGesture {
            // Don't start single-finger pan if long press or two-finger is active
            if isLongPressActive || isTwoFingerActive { return false }
            let velocity = panGesture.velocity(in: self)
            // Only claim horizontal pans (vertical → let ScrollView handle page scrolling)
            return abs(velocity.x) > abs(velocity.y) * 1.5
        }
        // Two-finger pan, pinch, long press: always begin
        return true
    }
}

// MARK: - Price Axis Drag Overlay (single-finger vertical drag on price axis for vertical zoom)

/// A UIView overlay placed on the price axis that captures single-finger vertical drags
/// for vertical zoom. It does NOT pass these touches to ScrollView.
///
/// Uses the same safe scroll-disable/enable pattern as `KLineGestureView`:
/// - Only captures the original scroll state on the first disable
/// - Uses a deferred restore timer as a safety net
class PriceAxisDragView: UIView, UIGestureRecognizerDelegate {
    var onVerticalDrag: ((UIPanGestureRecognizer) -> Void)?

    private weak var parentScrollView: UIScrollView?
    private var savedScrollEnabled: Bool = true
    private var savedBounces: Bool = true
    private var isDisabled: Bool = false
    private var deferredRestoreTimer: Timer?

    private lazy var verticalPanGesture: UIPanGestureRecognizer = {
        let g = UIPanGestureRecognizer(target: self, action: #selector(handleVerticalPan(_:)))
        g.delegate = self
        g.minimumNumberOfTouches = 1
        g.maximumNumberOfTouches = 1
        return g
    }()

    override init(frame: CGRect) {
        super.init(frame: frame)
        addGestureRecognizer(verticalPanGesture)
    }

    required init?(coder: NSCoder) {
        super.init(coder: coder)
        addGestureRecognizer(verticalPanGesture)
    }

    /// Safety net: when all fingers leave, force-restore scroll
    override func touchesEnded(_ touches: Set<UITouch>, with event: UIEvent?) {
        super.touchesEnded(touches, with: event)
        forceResetIfNoTouches(event: event)
    }

    override func touchesCancelled(_ touches: Set<UITouch>, with event: UIEvent?) {
        super.touchesCancelled(touches, with: event)
        forceResetIfNoTouches(event: event)
    }

    private func forceResetIfNoTouches(event: UIEvent?) {
        let remaining = event?.touches(for: self)?.filter {
            $0.phase == .began || $0.phase == .moved || $0.phase == .stationary
        }.count ?? 0

        if remaining == 0 && isDisabled {
            forceRestoreScroll()
        }
    }

    private func forceRestoreScroll() {
        deferredRestoreTimer?.invalidate()
        deferredRestoreTimer = nil
        isDisabled = false
        if let sv = parentScrollView {
            sv.isScrollEnabled = savedScrollEnabled
            sv.bounces = savedBounces
        }
    }

    private func findParentScrollView() -> UIScrollView? {
        var view: UIView? = self.superview
        while let v = view {
            if let scrollView = v as? UIScrollView {
                return scrollView
            }
            view = v.superview
        }
        return nil
    }

    private func disableParentScroll() {
        deferredRestoreTimer?.invalidate()
        deferredRestoreTimer = nil

        if parentScrollView == nil {
            parentScrollView = findParentScrollView()
        }
        if !isDisabled {
            // Only save original state on the first disable
            if let sv = parentScrollView {
                savedScrollEnabled = sv.isScrollEnabled
                savedBounces = sv.bounces
            }
            isDisabled = true
        }
        if let sv = parentScrollView {
            sv.isScrollEnabled = false
            sv.bounces = false
        }
    }

    private func enableParentScroll() {
        isDisabled = false
        if let sv = parentScrollView {
            sv.isScrollEnabled = savedScrollEnabled
            sv.bounces = savedBounces
        }
        // Schedule a deferred safety restore
        deferredRestoreTimer?.invalidate()
        deferredRestoreTimer = Timer.scheduledTimer(withTimeInterval: 0.15, repeats: false) { [weak self] _ in
            guard let self = self else { return }
            if self.verticalPanGesture.state != .began && self.verticalPanGesture.state != .changed {
                self.forceRestoreScroll()
            }
        }
    }

    @objc private func handleVerticalPan(_ gesture: UIPanGestureRecognizer) {
        switch gesture.state {
        case .began:
            disableParentScroll()
        case .changed:
            break
        case .ended, .cancelled, .failed:
            enableParentScroll()
        default:
            break
        }
        onVerticalDrag?(gesture)
    }

    /// Only begin if the gesture is primarily vertical (opposite of the chart area's horizontal pan)
    override func gestureRecognizerShouldBegin(_ gestureRecognizer: UIGestureRecognizer) -> Bool {
        if gestureRecognizer == verticalPanGesture {
            let velocity = verticalPanGesture.velocity(in: self)
            return abs(velocity.y) > abs(velocity.x) * 0.8
        }
        return true
    }
}

/// SwiftUI wrapper for PriceAxisDragView
struct PriceAxisDragOverlay: UIViewRepresentable {
    let chartHeight: CGFloat
    let chartState: KLineChartState

    func makeUIView(context: Context) -> PriceAxisDragView {
        let view = PriceAxisDragView()
        view.backgroundColor = .clear
        view.onVerticalDrag = { [weak chartState] gesture in
            context.coordinator.handleVerticalDrag(gesture, chartState: chartState, chartHeight: chartHeight)
        }
        return view
    }

    func updateUIView(_ uiView: PriceAxisDragView, context: Context) {
        uiView.onVerticalDrag = { [weak chartState] gesture in
            context.coordinator.handleVerticalDrag(gesture, chartState: chartState, chartHeight: chartHeight)
        }
    }

    func makeCoordinator() -> Coordinator {
        Coordinator()
    }

    class Coordinator {
        private var lastDragY: CGFloat = 0

        func handleVerticalDrag(_ gesture: UIPanGestureRecognizer, chartState: KLineChartState?, chartHeight: CGFloat) {
            guard let state = chartState else { return }
            if state.isTouching { return }

            switch gesture.state {
            case .began:
                state.verticalDragBaseScale = state.verticalScale
                state.isVerticalDragging = true
                lastDragY = 0
            case .changed:
                let translation = gesture.translation(in: gesture.view).y
                let delta = translation - lastDragY
                lastDragY = translation

                // Dragging up (negative Y) = zoom in (increase scale)
                // Dragging down (positive Y) = zoom out (decrease scale)
                let sensitivity: CGFloat = 2.0 / max(1, chartHeight)
                let scaleDelta = -delta * sensitivity * state.verticalScale
                let newScale = state.verticalScale + scaleDelta
                let clamped = min(state.maxVerticalScale, max(state.minVerticalScale, newScale))
                DispatchQueue.main.async {
                    state.verticalScale = clamped
                }
            case .ended, .cancelled:
                state.isVerticalDragging = false
                lastDragY = 0
            default:
                break
            }
        }
    }
}

/// SwiftUI wrapper for KLineGestureView
struct KLineGestureOverlay: UIViewRepresentable {
    let chartWidth: CGFloat
    let chartHeight: CGFloat
    let chartState: KLineChartState
    let dataCount: Int
    let visibleRange: Range<Int>
    let initialRightMargin: CGFloat
    let onScrollChanged: () -> Void
    let onLoadMoreCheck: () -> Void

    func makeUIView(context: Context) -> KLineGestureView {
        let view = KLineGestureView()
        view.backgroundColor = .clear
        view.onPan = { [weak chartState] gesture in
            context.coordinator.handlePan(gesture, chartState: chartState, chartWidth: chartWidth,
                                          dataCount: dataCount, initialRightMargin: initialRightMargin,
                                          onScrollChanged: onScrollChanged,
                                          onLoadMoreCheck: onLoadMoreCheck)
        }
        view.onPinch = { [weak chartState] gesture in
            context.coordinator.handlePinch(gesture, chartState: chartState, onLoadMoreCheck: onLoadMoreCheck)
        }
        view.onTwoFingerPan = { [weak chartState] gesture in
            context.coordinator.handleTwoFingerPan(gesture, chartState: chartState, chartHeight: chartHeight)
        }
        view.onLongPress = { [weak chartState] gesture in
            context.coordinator.handleLongPress(gesture, chartState: chartState, chartWidth: chartWidth,
                                                visibleRange: visibleRange)
        }
        return view
    }

    func updateUIView(_ uiView: KLineGestureView, context: Context) {
        uiView.onPan = { [weak chartState] gesture in
            context.coordinator.handlePan(gesture, chartState: chartState, chartWidth: chartWidth,
                                          dataCount: dataCount, initialRightMargin: initialRightMargin,
                                          onScrollChanged: onScrollChanged,
                                          onLoadMoreCheck: onLoadMoreCheck)
        }
        uiView.onPinch = { [weak chartState] gesture in
            context.coordinator.handlePinch(gesture, chartState: chartState, onLoadMoreCheck: onLoadMoreCheck)
        }
        uiView.onTwoFingerPan = { [weak chartState] gesture in
            context.coordinator.handleTwoFingerPan(gesture, chartState: chartState, chartHeight: chartHeight)
        }
        uiView.onLongPress = { [weak chartState] gesture in
            context.coordinator.handleLongPress(gesture, chartState: chartState, chartWidth: chartWidth,
                                                visibleRange: visibleRange)
        }
    }

    func makeCoordinator() -> Coordinator {
        Coordinator()
    }

    class Coordinator {
        private var lastPanTranslation: CGFloat = 0
        private var lastTwoFingerY: CGFloat = 0

        func handlePan(_ gesture: UIPanGestureRecognizer, chartState: KLineChartState?,
                        chartWidth: CGFloat, dataCount: Int,
                        initialRightMargin: CGFloat,
                        onScrollChanged: @escaping () -> Void,
                        onLoadMoreCheck: @escaping () -> Void) {
            guard let state = chartState else { return }
            if state.isTouching { return } // Don't pan during crosshair

            switch gesture.state {
            case .began:
                // Stop any running inertia animation when the user touches again
                state.stopMomentum()
                lastPanTranslation = 0
            case .changed:
                let translation = gesture.translation(in: gesture.view).x
                let delta = translation - lastPanTranslation
                lastPanTranslation = translation

                // Use the shared two-layer scrolling logic
                DispatchQueue.main.async {
                    state.applyPixelDelta(delta,
                                          chartWidth: chartWidth,
                                          dataCount: dataCount,
                                          initialRightMargin: initialRightMargin)
                    onScrollChanged()
                    onLoadMoreCheck()
                }
            case .ended, .cancelled:
                // Capture velocity from the gesture recognizer (pixels/second)
                let velocity = gesture.velocity(in: gesture.view).x

                state.dragAccumulator = 0
                lastPanTranslation = 0

                // Start inertia animation if the flick was fast enough
                DispatchQueue.main.async {
                    state.startMomentum(
                        velocity: velocity,
                        chartWidth: chartWidth,
                        dataCount: dataCount,
                        initialRightMargin: initialRightMargin,
                        onScrollChanged: onScrollChanged,
                        onLoadMoreCheck: onLoadMoreCheck
                    )
                }
            default:
                break
            }
        }

        func handlePinch(_ gesture: UIPinchGestureRecognizer, chartState: KLineChartState?,
                          onLoadMoreCheck: @escaping () -> Void) {
            guard let state = chartState else { return }
            if state.isTouching { return }

            switch gesture.state {
            case .began:
                state.pinchBaseVisibleCount = state.visibleCount
                state.isPinching = true
            case .changed:
                let scale = gesture.scale
                guard scale > 0 else { return }
                let newCount = Double(state.pinchBaseVisibleCount) / Double(scale)
                let clamped = min(state.maxVisibleCount, max(state.minVisibleCount, Int(newCount)))
                DispatchQueue.main.async {
                    state.visibleCount = clamped
                    onLoadMoreCheck()
                }
            case .ended, .cancelled:
                state.isPinching = false
            default:
                break
            }
        }

        /// Two-finger vertical pan → vertical zoom (replaces PriceAxisGestureView functionality)
        func handleTwoFingerPan(_ gesture: UIPanGestureRecognizer, chartState: KLineChartState?, chartHeight: CGFloat) {
            guard let state = chartState else { return }
            if state.isTouching { return }

            switch gesture.state {
            case .began:
                state.verticalDragBaseScale = state.verticalScale
                state.isVerticalDragging = true
                lastTwoFingerY = 0
            case .changed:
                let translation = gesture.translation(in: gesture.view).y
                let delta = translation - lastTwoFingerY
                lastTwoFingerY = translation

                // Dragging up (negative Y) = zoom in (increase scale)
                // Dragging down (positive Y) = zoom out (decrease scale)
                let sensitivity: CGFloat = 2.0 / max(1, chartHeight)
                let scaleDelta = -delta * sensitivity * state.verticalScale
                let newScale = state.verticalScale + scaleDelta
                let clamped = min(state.maxVerticalScale, max(state.minVerticalScale, newScale))
                DispatchQueue.main.async {
                    state.verticalScale = clamped
                }
            case .ended, .cancelled:
                state.isVerticalDragging = false
                lastTwoFingerY = 0
            default:
                break
            }
        }

        func handleLongPress(_ gesture: UILongPressGestureRecognizer, chartState: KLineChartState?,
                              chartWidth: CGFloat, visibleRange: Range<Int>) {
            guard let state = chartState else { return }
            let count = visibleRange.upperBound - visibleRange.lowerBound
            guard count > 0, chartWidth > 0 else { return }

            switch gesture.state {
            case .began, .changed:
                let location = gesture.location(in: gesture.view)
                let spacing = chartWidth / CGFloat(count)
                let localIndex = Int(location.x / spacing)
                let clampedLocal = max(0, min(count - 1, localIndex))
                DispatchQueue.main.async {
                    state.isTouching = true
                    state.crosshairIndex = visibleRange.lowerBound + clampedLocal
                }
            case .ended, .cancelled:
                DispatchQueue.main.async {
                    state.isTouching = false
                    state.crosshairIndex = nil
                }
            default:
                break
            }
        }
    }
}

// MARK: - KLineChartView

/// K-Line (candlestick) chart view with pan, pinch-to-zoom, crosshair, and pagination
struct KLineChartView: View {
    let data: [KLinePoint]
    let accentColor: Color
    /// Callback when more historical data is needed
    var onLoadMore: (() -> Void)? = nil
    /// Whether additional data is currently being loaded
    var isLoadingMore: Bool = false
    /// The currently selected K-line period (for time axis formatting)
    var period: String = "1d"

    @StateObject private var chartState = KLineChartState()

    /// Width reserved for the price axis on the right
    private let priceAxisWidth: CGFloat = 55

    /// Initial right margin when showing the latest data (scrollOffset == 0).
    /// The last candle starts offset by this amount from the right edge, but
    /// the user can scroll right to fill this space — it does NOT create a
    /// permanent dead zone.
    private let initialRightMargin: CGFloat = 30

    // Computed visible window
    private var visibleRange: Range<Int> {
        let count = data.count
        guard count > 0 else { return 0..<0 }
        let visible = min(chartState.visibleCount, count)
        let maxOffset = max(0, count - visible)
        let offset = min(chartState.scrollOffset, maxOffset)
        let endIndex = count - offset
        let startIndex = max(0, endIndex - visible)
        return startIndex..<endIndex
    }

    private var visibleData: [KLinePoint] {
        guard !data.isEmpty else { return [] }
        return Array(data[visibleRange])
    }

    /// Price range for visible data (with vertical scale applied and padding for labels)
    private func priceExtent(for vd: [KLinePoint]) -> (min: Double, max: Double, range: Double) {
        guard let rawMin = vd.map({ $0.low }).min(),
              let rawMax = vd.map({ $0.high }).max(),
              rawMax > rawMin else {
            return (0, 1, 1)
        }

        let rawRange = rawMax - rawMin
        let midPrice = (rawMax + rawMin) / 2.0

        // Apply vertical scale: smaller range = more zoomed in
        let scaledHalf = (rawRange / 2.0) / Double(chartState.verticalScale)
        let offsetShift = Double(chartState.verticalOffset) * rawRange

        let newMin = midPrice - scaledHalf + offsetShift
        let newMax = midPrice + scaledHalf + offsetShift
        let scaledRange = newMax - newMin

        // Add 5% padding on each side so edge labels (high/low) are not clipped
        let padding = scaledRange * 0.05
        let paddedMin = newMin - padding
        let paddedMax = newMax + padding
        return (paddedMin, paddedMax, paddedMax - paddedMin)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            // Header: selected point info or summary
            if let idx = chartState.crosshairIndex, idx >= 0, idx < data.count {
                crosshairInfo(data[idx])
            } else {
                chartSummary
            }

            // Chart area with price axis
            GeometryReader { geometry in
                let totalWidth = geometry.size.width
                // fullChartWidth = total chart drawing area (everything except price axis)
                let fullChartWidth = totalWidth - priceAxisWidth
                // candleAreaWidth = the width used to calculate candle spacing / positions
                // This is the full chart width — candles CAN draw all the way to the right edge.
                let candleAreaWidth = fullChartWidth
                let height = geometry.size.height

                // Dynamic right offset: controlled by pixelRightMargin in chartState.
                // Initially 30px (newest candle is offset from right edge).
                // User can scroll right to reduce this margin to 0.
                let rightOffset = chartState.pixelRightMargin

                HStack(spacing: 0) {
                    // Main chart area (candlesticks + labels + crosshair + gesture overlay)
                    ZStack(alignment: .topLeading) {
                        // Background grid lines
                        chartBackground(width: fullChartWidth, height: height)

                        // Candlesticks
                        candlesticksLayer(width: candleAreaWidth, height: height, rightOffset: rightOffset)

                        // Price labels: current price + high/low labels
                        priceLabelsOverlay(width: candleAreaWidth, fullWidth: fullChartWidth, height: height, rightOffset: rightOffset)

                        // Crosshair overlay
                        if chartState.isTouching, let idx = chartState.crosshairIndex,
                           idx >= visibleRange.lowerBound, idx < visibleRange.upperBound {
                            crosshairOverlay(
                                index: idx,
                                chartWidth: candleAreaWidth,
                                height: height,
                                rightOffset: rightOffset
                            )
                        }

                        // UIKit gesture overlay (captures pan, pinch, two-finger-pan, long-press)
                        KLineGestureOverlay(
                            chartWidth: candleAreaWidth,
                            chartHeight: height,
                            chartState: chartState,
                            dataCount: data.count,
                            visibleRange: visibleRange,
                            initialRightMargin: initialRightMargin,
                            onScrollChanged: {},
                            onLoadMoreCheck: { checkLoadMore() }
                        )
                    }
                    .frame(width: fullChartWidth)

                    // Price axis on the right (with vertical drag overlay for zoom)
                    ZStack {
                        priceAxisView(height: height)
                        PriceAxisDragOverlay(
                            chartHeight: height,
                            chartState: chartState
                        )
                    }
                    .frame(width: priceAxisWidth)
                }
            }
            .frame(height: 220)

            // Time axis labels
            timeAxisLabels
        }
        .padding(16)
        .background(Color.secondaryBackground)
        .cornerRadius(16)
        .onAppear {
            chartState.currentPeriod = period
        }
        .onChange(of: data.count) { _ in
            // When data grows (e.g. more loaded), allow requesting more again
            chartState.hasRequestedMore = false
        }
        .onChange(of: period) { newPeriod in
            chartState.currentPeriod = newPeriod
        }
    }

    // MARK: - Chart Background & Grid

    private func chartBackground(width: CGFloat, height: CGFloat) -> some View {
        Canvas { context, size in
            // Draw 4 horizontal grid lines
            for i in 1..<5 {
                let y = height * CGFloat(i) / 5.0
                var path = Path()
                path.move(to: CGPoint(x: 0, y: y))
                path.addLine(to: CGPoint(x: width, y: y))
                context.stroke(path, with: .color(.white.opacity(0.06)), lineWidth: 0.5)
            }
        }
    }

    // MARK: - Price Axis View (with current price label)

    private func priceAxisView(height: CGFloat) -> some View {
        let vd = visibleData
        let extent = priceExtent(for: vd)
        let currentPrice = data.last?.close ?? 0

        return Canvas { context, size in
            guard extent.range > 0 else { return }

            // Draw 5 price labels evenly spaced
            for i in 0..<5 {
                let price = extent.max - extent.range * Double(i) / 4.0
                let y = height * CGFloat(i) / 4.0 + 4
                let text = Text(formatPrice(price))
                    .font(.system(size: 9, design: .monospaced))
                    .foregroundColor(.textTertiary.opacity(0.7))
                context.draw(context.resolve(text), at: CGPoint(x: priceAxisWidth / 2, y: y), anchor: .top)
            }

            // Current price label — colored pill on the price axis
            if currentPrice > 0 {
                let priceY = (1 - CGFloat((currentPrice - extent.min) / extent.range)) * height
                let clampedY = max(8, min(height - 8, priceY))
                let isUp = data.last?.isUp ?? true
                let bgColor = isUp ? Color.accentGreen : Color(hex: "E53935")

                // Draw background pill (compact width to avoid crowding the chart)
                let pillWidth: CGFloat = priceAxisWidth - 8
                let pillHeight: CGFloat = 14
                let pillRect = CGRect(x: 4, y: clampedY - pillHeight / 2, width: pillWidth, height: pillHeight)
                context.fill(
                    Path(roundedRect: pillRect, cornerRadius: 3),
                    with: .color(bgColor)
                )

                // Draw price text
                let priceText = Text(formatPrice(currentPrice))
                    .font(.system(size: 7, weight: .bold, design: .monospaced))
                    .foregroundColor(.white)
                context.draw(context.resolve(priceText), at: CGPoint(x: priceAxisWidth / 2, y: clampedY), anchor: .center)
            }
        }
    }

    // MARK: - Candlesticks Layer

    private func candlesticksLayer(width: CGFloat, height: CGFloat, rightOffset: CGFloat) -> some View {
        let vd = visibleData
        let count = vd.count
        guard count > 0 else { return AnyView(EmptyView()) }

        let extent = priceExtent(for: vd)
        let candleSpacing = width / CGFloat(count)
        let candleBodyWidth = max(1.5, candleSpacing * 0.7)

        return AnyView(
            Canvas { context, size in
                for (i, point) in vd.enumerated() {
                    let centerX = candleSpacing * (CGFloat(i) + 0.5) - rightOffset
                    let color = point.isUp ? Color.accentGreen : Color(hex: "E53935")

                    // Map price to Y coordinate
                    func priceToY(_ price: Double) -> CGFloat {
                        guard extent.range > 0 else { return height / 2 }
                        return (1 - CGFloat((price - extent.min) / extent.range)) * height
                    }

                    // Wick (shadow line)
                    let highY = priceToY(point.high)
                    let lowY = priceToY(point.low)
                    let wickWidth: CGFloat = max(0.8, candleBodyWidth > 4 ? 1.0 : 0.8)
                    context.fill(
                        Path(CGRect(
                            x: centerX - wickWidth / 2,
                            y: min(highY, lowY),
                            width: wickWidth,
                            height: max(1, abs(lowY - highY))
                        )),
                        with: .color(color)
                    )

                    // Body
                    let bodyTop = max(point.open, point.close)
                    let bodyBottom = min(point.open, point.close)
                    let bodyTopY = priceToY(bodyTop)
                    let bodyBottomY = priceToY(bodyBottom)
                    let bodyHeight = max(1, bodyBottomY - bodyTopY)

                    context.fill(
                        Path(CGRect(
                            x: centerX - candleBodyWidth / 2,
                            y: bodyTopY,
                            width: candleBodyWidth,
                            height: bodyHeight
                        )),
                        with: .color(color)
                    )
                }
            }
        )
    }

    // MARK: - Price Labels Overlay (current price dashed line + high/low labels)

    private func priceLabelsOverlay(width: CGFloat, fullWidth: CGFloat, height: CGFloat, rightOffset: CGFloat) -> some View {
        let vd = visibleData
        let count = vd.count
        guard count > 0 else { return AnyView(EmptyView()) }

        let extent = priceExtent(for: vd)
        guard extent.range > 0 else { return AnyView(EmptyView()) }

        let candleSpacing = width / CGFloat(count)

        // Find highest and lowest points in visible data
        var highIdx = 0, lowIdx = 0
        var highPrice = vd[0].high, lowPrice = vd[0].low
        for (i, p) in vd.enumerated() {
            if p.high > highPrice { highPrice = p.high; highIdx = i }
            if p.low < lowPrice { lowPrice = p.low; lowIdx = i }
        }

        let highCenterX = candleSpacing * (CGFloat(highIdx) + 0.5) - rightOffset
        let lowCenterX = candleSpacing * (CGFloat(lowIdx) + 0.5) - rightOffset
        // Clamp Y positions so labels are never cut off at edges
        let highYRaw = (1 - CGFloat((highPrice - extent.min) / extent.range)) * height
        let lowYRaw = (1 - CGFloat((lowPrice - extent.min) / extent.range)) * height
        let highY = max(8, min(height - 8, highYRaw))
        let lowY = max(8, min(height - 8, lowYRaw))

        // Current (latest) price
        let currentPrice = data.last?.close ?? 0
        let currentPriceY = currentPrice > 0
            ? (1 - CGFloat((currentPrice - extent.min) / extent.range)) * height
            : CGFloat(-100) // off screen if no price

        // Check if the last candle is within the visible range
        let lastCandleVisible = chartState.scrollOffset == 0

        // X position of the last visible candle's center (with offset applied)
        let lastVisibleCandleX = candleSpacing * (CGFloat(count) - 0.5) - rightOffset

        // Determine label placement for high/low to avoid overlap:
        // - If high is in right half → label goes left; else right
        // - If low is in right half → label goes left; else right
        let highOnRight = highCenterX < width * 0.6
        let lowOnRight = lowCenterX < width * 0.6

        return AnyView(
            Canvas { context, size in
                // 1. Current price dashed line
                // When the current candle is visible, only draw from the candle position to the right edge.
                // When scrolled away (not visible), draw across the full width.
                if currentPrice > 0 {
                    let clampedPriceY = max(0, min(height, currentPriceY))
                    var dashedPath = Path()

                    if lastCandleVisible {
                        // Only draw from the last candle to the right edge (into the padding area)
                        dashedPath.move(to: CGPoint(x: lastVisibleCandleX, y: clampedPriceY))
                        dashedPath.addLine(to: CGPoint(x: fullWidth, y: clampedPriceY))
                    } else {
                        // Current candle is off-screen — draw full-width dashed line
                        dashedPath.move(to: CGPoint(x: 0, y: clampedPriceY))
                        dashedPath.addLine(to: CGPoint(x: fullWidth, y: clampedPriceY))
                    }

                    let isUp = data.last?.isUp ?? true
                    let lineColor = isUp ? Color.accentGreen.opacity(0.6) : Color(hex: "E53935").opacity(0.6)
                    context.stroke(dashedPath, with: .color(lineColor),
                                   style: StrokeStyle(lineWidth: 0.5, dash: [4, 3]))
                }

                // 2. Highest price label
                let highText = Text(formatPrice(highPrice))
                    .font(.system(size: 8, weight: .medium, design: .monospaced))
                    .foregroundColor(.accentGreen)
                let highLabelX = highOnRight
                    ? highCenterX + candleSpacing * 0.5 + 2
                    : highCenterX - candleSpacing * 0.5 - 2
                let highAnchor: UnitPoint = highOnRight ? .leading : .trailing

                // Small line from candle top to label
                var highLine = Path()
                highLine.move(to: CGPoint(x: highCenterX, y: highY))
                highLine.addLine(to: CGPoint(x: highLabelX, y: highY))
                context.stroke(highLine, with: .color(Color.accentGreen.opacity(0.5)), lineWidth: 0.5)
                context.draw(context.resolve(highText),
                             at: CGPoint(x: highLabelX, y: highY - 1),
                             anchor: highAnchor)

                // 3. Lowest price label
                let lowText = Text(formatPrice(lowPrice))
                    .font(.system(size: 8, weight: .medium, design: .monospaced))
                    .foregroundColor(Color(hex: "E53935"))
                let lowLabelX = lowOnRight
                    ? lowCenterX + candleSpacing * 0.5 + 2
                    : lowCenterX - candleSpacing * 0.5 - 2
                let lowAnchor: UnitPoint = lowOnRight ? .leading : .trailing

                var lowLine = Path()
                lowLine.move(to: CGPoint(x: lowCenterX, y: lowY))
                lowLine.addLine(to: CGPoint(x: lowLabelX, y: lowY))
                context.stroke(lowLine, with: .color(Color(hex: "E53935").opacity(0.5)), lineWidth: 0.5)
                context.draw(context.resolve(lowText),
                             at: CGPoint(x: lowLabelX, y: lowY + 1),
                             anchor: lowAnchor)
            }
        )
    }

    // MARK: - Crosshair Overlay

    private func crosshairOverlay(index: Int, chartWidth: CGFloat, height: CGFloat, rightOffset: CGFloat) -> some View {
        let vd = visibleData
        let count = vd.count
        guard count > 0 else { return AnyView(EmptyView()) }

        let localIndex = index - visibleRange.lowerBound
        guard localIndex >= 0, localIndex < count else { return AnyView(EmptyView()) }

        let point = data[index]
        let spacing = chartWidth / CGFloat(count)
        let centerX = spacing * (CGFloat(localIndex) + 0.5) - rightOffset
        let extent = priceExtent(for: vd)

        let closeY: CGFloat = extent.range > 0
            ? (1 - CGFloat((point.close - extent.min) / extent.range)) * height
            : height / 2

        return AnyView(
            ZStack {
                // Vertical line
                Path { path in
                    path.move(to: CGPoint(x: centerX, y: 0))
                    path.addLine(to: CGPoint(x: centerX, y: height))
                }
                .stroke(Color.white.opacity(0.5), style: StrokeStyle(lineWidth: 0.5, dash: [4, 3]))

                // Horizontal line
                Path { path in
                    path.move(to: CGPoint(x: 0, y: closeY))
                    path.addLine(to: CGPoint(x: chartWidth, y: closeY))
                }
                .stroke(Color.white.opacity(0.5), style: StrokeStyle(lineWidth: 0.5, dash: [4, 3]))

                // Price label on left
                Text(formatPrice(point.close))
                    .font(.system(size: 9, weight: .medium, design: .monospaced))
                    .foregroundColor(.white)
                    .padding(.horizontal, 4)
                    .padding(.vertical, 2)
                    .background(accentColor.opacity(0.9))
                    .cornerRadius(3)
                    .position(x: 28, y: closeY)

                // Dot at crosshair intersection
                Circle()
                    .fill(Color.white)
                    .frame(width: 6, height: 6)
                    .shadow(color: accentColor, radius: 3)
                    .position(x: centerX, y: closeY)
            }
        )
    }

    // MARK: - Pagination

    /// Check if we should request more historical data.
    /// Uses hasRequestedMore as debounce: set on request, cleared when data.count changes
    /// or auto-cleared after 3 seconds (fallback for when API has no more data).
    private func checkLoadMore() {
        guard !chartState.hasRequestedMore, onLoadMore != nil else { return }

        let maxOffset = max(0, data.count - chartState.visibleCount)
        let currentOffset = chartState.scrollOffset

        // remaining = candles left before reaching the oldest loaded data
        let remaining = maxOffset - currentOffset

        // Trigger preload when fewer than threshold candles remain on the left
        let threshold = min(200, max(30, data.count / 3))
        if remaining < threshold {
            chartState.hasRequestedMore = true
            onLoadMore?()

            // Auto-reset after 3s in case data.count doesn't change (API has no more data)
            DispatchQueue.main.asyncAfter(deadline: .now() + 3.0) { [weak chartState] in
                chartState?.hasRequestedMore = false
            }
        }
    }

    // MARK: - Time Axis Labels

    private var timeAxisLabels: some View {
        let vd = visibleData
        guard vd.count > 2 else { return AnyView(EmptyView()) }

        let formatter = DateFormatter()
        let p = chartState.currentPeriod

        // Determine format and max label count based on period:
        // - Minute-level: "M/d HH:mm" (compact, max 3 labels to avoid overlap)
        // - Daily: "yyyy-M-d" (full date, max 4 labels)
        // - Weekly: "yyyy-M-d" (full date, max 4 labels)
        let maxLabelCount: Int
        switch p {
        case "5m", "15m", "30m", "1h", "4h":
            // Minute-level: show month/day + hour:minute
            // Use compact format to fit in one line
            formatter.dateFormat = "M/d HH:mm"
            maxLabelCount = 3  // Fewer labels since each label is wider
        case "1d":
            // Daily: show full year-month-day
            formatter.dateFormat = "yyyy-M-d"
            maxLabelCount = 4
        case "1w":
            // Weekly: show full year-month-day
            formatter.dateFormat = "yyyy-M-d"
            maxLabelCount = 4
        default:
            formatter.dateFormat = "yyyy-M-d"
            maxLabelCount = 4
        }

        // Calculate labels: evenly spaced, respecting maxLabelCount
        let labelCount = min(maxLabelCount, vd.count)
        let step = max(1, vd.count / labelCount)

        var indices: [Int] = []
        var i = 0
        while i < vd.count {
            indices.append(i)
            i += step
        }
        // Always include the last data point
        if let last = indices.last, last != vd.count - 1 {
            indices.append(vd.count - 1)
        }

        return AnyView(
            HStack {
                ForEach(indices, id: \.self) { idx in
                    if idx == indices.first {
                        Text(formatter.string(from: vd[idx].date))
                            .font(.system(size: 8, design: .monospaced))
                            .foregroundColor(.textTertiary)
                            .lineLimit(1)
                            .fixedSize(horizontal: true, vertical: false)
                    }
                    if idx != indices.first {
                        Spacer()
                        Text(formatter.string(from: vd[idx].date))
                            .font(.system(size: 8, design: .monospaced))
                            .foregroundColor(.textTertiary)
                            .lineLimit(1)
                            .fixedSize(horizontal: true, vertical: false)
                    }
                }
                // Add spacer matching price axis width
                Color.clear.frame(width: priceAxisWidth - 16)
            }
        )
    }

    // MARK: - Header Views

    private func crosshairInfo(_ point: KLinePoint) -> some View {
        let dateFormatter = DateFormatter()
        let calendar = Calendar.current
        let hasTime = calendar.component(.hour, from: point.date) != 0 || calendar.component(.minute, from: point.date) != 0
        dateFormatter.dateFormat = hasTime ? "yyyy年M月d日 HH:mm" : "yyyy年M月d日"

        return HStack(spacing: 12) {
            VStack(alignment: .leading, spacing: 2) {
                Text(dateFormatter.string(from: point.date))
                    .font(.system(size: 12))
                    .foregroundColor(.textSecondary)
                HStack(spacing: 10) {
                    infoLabel("开", value: formatPrice(point.open), color: .textPrimary)
                    infoLabel("收", value: formatPrice(point.close), color: point.isUp ? .accentGreen : Color(hex: "E53935"))
                    infoLabel("高", value: formatPrice(point.high), color: .accentGreen)
                    infoLabel("低", value: formatPrice(point.low), color: Color(hex: "E53935"))
                }
            }
            Spacer()
        }
    }

    private func infoLabel(_ label: String, value: String, color: Color) -> some View {
        HStack(spacing: 2) {
            Text(label)
                .font(.system(size: 10))
                .foregroundColor(.textTertiary)
            Text(value)
                .font(.system(size: 11, weight: .medium, design: .monospaced))
                .foregroundColor(color)
        }
    }

    private var chartSummary: some View {
        HStack {
            Text("K线走势")
                .font(.system(size: 14, weight: .semibold))
                .foregroundColor(.textPrimary)
            Spacer()
            if !visibleData.isEmpty {
                let last = visibleData.last!
                let first = visibleData.first!
                let change = last.close - first.open
                let pct = first.open > 0 ? change / first.open * 100 : 0
                Text("\(change >= 0 ? "+" : "")\(String(format: "%.2f", pct))%")
                    .font(.system(size: 12, weight: .medium, design: .monospaced))
                    .foregroundColor(change >= 0 ? .accentGreen : Color(hex: "E53935"))
            }
        }
    }

    // MARK: - Helpers

    private func formatPrice(_ price: Double) -> String {
        if price >= 10000 {
            return String(format: "%.0f", price)
        } else if price >= 1000 {
            return String(format: "%.1f", price)
        } else if price >= 1 {
            return String(format: "%.2f", price)
        } else {
            return String(format: "%.4f", price)
        }
    }
}
