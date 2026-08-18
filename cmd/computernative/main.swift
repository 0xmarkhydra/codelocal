import Foundation
import AppKit
import ApplicationServices
import ScreenCaptureKit
import Vision
import ImageIO
import UniformTypeIdentifiers

private let protocolVersion = 1

private final class SceneEventStore {
    static let shared = SceneEventStore()
    private let lock = NSLock()
    private var sequence: UInt64 = 0
    private var events: [[String: Any]] = []

    func append(pid: pid_t, kind: String, windowID: String? = nil) {
        lock.lock()
        defer { lock.unlock() }
        sequence &+= 1
        var event: [String: Any] = [
            "sequence": sequence,
            "pid": Int(pid),
            "kind": kind,
            "timestampMs": Int(Date().timeIntervalSince1970 * 1000),
        ]
        if let windowID, !windowID.isEmpty { event["windowId"] = windowID }
        events.append(event)
        if events.count > 256 {
            events.removeFirst(events.count - 256)
        }
    }

    func drain() -> [String: Any] {
        lock.lock()
        defer { lock.unlock() }
        let drained = events
        events.removeAll(keepingCapacity: true)
        return ["sequence": sequence, "events": drained]
    }
}

private final class ObserverContext {
    let pid: pid_t
    init(pid: pid_t) { self.pid = pid }
}

private struct ObserverHandle {
    let observer: AXObserver
    let context: ObserverContext
}

private let windowNotifications = [
    kAXValueChangedNotification,
    kAXMovedNotification,
    kAXResizedNotification,
    kAXTitleChangedNotification,
    kAXUIElementDestroyedNotification,
]

private func nativeWindowID(pid: pid_t, element: AXUIElement) -> String? {
    let app = AXUIElementCreateApplication(pid)
    for (index, window) in axElements(app, kAXWindowsAttribute).enumerated() where CFEqual(window, element) {
        return "ax:\(pid):\(index)"
    }
    return nil
}

private func observeWindow(_ observer: AXObserver, _ window: AXUIElement, _ refcon: UnsafeMutableRawPointer) {
    for notification in windowNotifications {
        _ = AXObserverAddNotification(observer, window, notification as CFString, refcon)
    }
}

private let observerCallback: AXObserverCallback = { observer, element, notification, refcon in
    guard let refcon else { return }
    let context = Unmanaged<ObserverContext>.fromOpaque(refcon).takeUnretainedValue()
    let kind = notification as String
    if kind == kAXWindowCreatedNotification as String {
        observeWindow(observer, element, refcon)
    }
    SceneEventStore.shared.append(pid: context.pid, kind: kind, windowID: nativeWindowID(pid: context.pid, element: element))
}

private final class ObserverRegistry {
    static let shared = ObserverRegistry()
    private let lock = NSLock()
    private var handles: [pid_t: ObserverHandle] = [:]

    func ensure(pid: pid_t) {
        lock.lock()
        if handles[pid] != nil {
            lock.unlock()
            return
        }
        lock.unlock()

        var observer: AXObserver?
        guard AXObserverCreate(pid, observerCallback, &observer) == .success, let observer else { return }
        let context = ObserverContext(pid: pid)
        let refcon = Unmanaged.passUnretained(context).toOpaque()
        let app = AXUIElementCreateApplication(pid)
        let notifications = [
            kAXWindowCreatedNotification,
            kAXFocusedWindowChangedNotification,
            kAXUIElementDestroyedNotification,
        ]
        for notification in notifications {
            _ = AXObserverAddNotification(observer, app, notification as CFString, refcon)
        }
        for window in axElements(app, kAXWindowsAttribute) {
            observeWindow(observer, window, refcon)
        }

        let source = AXObserverGetRunLoopSource(observer)
        Thread.detachNewThread {
            CFRunLoopAddSource(CFRunLoopGetCurrent(), source, .defaultMode)
            CFRunLoopRun()
        }

        lock.lock()
        handles[pid] = ObserverHandle(observer: observer, context: context)
        lock.unlock()
    }
}

private func axAttribute(_ element: AXUIElement, _ name: String) -> CFTypeRef? {
    var value: CFTypeRef?
    guard AXUIElementCopyAttributeValue(element, name as CFString, &value) == .success else { return nil }
    return value
}

private func axElements(_ element: AXUIElement, _ name: String) -> [AXUIElement] {
    guard let value = axAttribute(element, name) else { return [] }
    return value as? [AXUIElement] ?? []
}

private func axString(_ element: AXUIElement, _ name: String) -> String {
    guard let value = axAttribute(element, name) else { return "" }
    if let text = value as? String { return text }
    if let number = value as? NSNumber { return number.stringValue }
    return ""
}

private func axBool(_ element: AXUIElement, _ name: String, fallback: Bool = true) -> Bool {
    guard let value = axAttribute(element, name) else { return fallback }
    if let flag = value as? Bool { return flag }
    if let number = value as? NSNumber { return number.boolValue }
    return fallback
}

private func axPoint(_ element: AXUIElement, _ name: String) -> CGPoint? {
    guard let value = axAttribute(element, name), CFGetTypeID(value) == AXValueGetTypeID() else { return nil }
    let axValue = unsafeBitCast(value, to: AXValue.self)
    guard AXValueGetType(axValue) == .cgPoint else { return nil }
    var point = CGPoint.zero
    return AXValueGetValue(axValue, .cgPoint, &point) ? point : nil
}

private func axSize(_ element: AXUIElement, _ name: String) -> CGSize? {
    guard let value = axAttribute(element, name), CFGetTypeID(value) == AXValueGetTypeID() else { return nil }
    let axValue = unsafeBitCast(value, to: AXValue.self)
    guard AXValueGetType(axValue) == .cgSize else { return nil }
    var size = CGSize.zero
    return AXValueGetValue(axValue, .cgSize, &size) ? size : nil
}

private func bounds(_ element: AXUIElement) -> [String: Double]? {
    guard let point = axPoint(element, kAXPositionAttribute), let size = axSize(element, kAXSizeAttribute) else { return nil }
    return ["x": point.x, "y": point.y, "width": size.width, "height": size.height]
}

private func safeValue(_ element: AXUIElement, role: String) -> Any? {
    if role.lowercased().contains("secure") { return nil }
    guard let value = axAttribute(element, kAXValueAttribute) else { return nil }
    if let text = value as? String { return text }
    if let number = value as? NSNumber { return number }
    return nil
}

private func runningAppName(pid: pid_t) -> String {
    NSRunningApplication(processIdentifier: pid)?.localizedName ?? ""
}

private func windows() throws -> [[String: Any]] {
    guard AXIsProcessTrusted() else { throw NativeError.message("macOS Accessibility permission is required") }
    var output: [[String: Any]] = []
    for app in NSWorkspace.shared.runningApplications where app.processIdentifier > 0 && app.activationPolicy == .regular {
        let pid = app.processIdentifier
        let appElement = AXUIElementCreateApplication(pid)
        let appWindows = axElements(appElement, kAXWindowsAttribute)
        if appWindows.isEmpty { continue }
        ObserverRegistry.shared.ensure(pid: pid)
        for (index, window) in appWindows.enumerated() {
            let size = axSize(window, kAXSizeAttribute)
            guard let size, size.width > 1, size.height > 1 else { continue }
            var item: [String: Any] = [
                "windowId": "ax:\(pid):\(index)",
                "pid": Int(pid),
                "app": app.localizedName ?? "",
                "title": axString(window, kAXTitleAttribute),
                "backend": "native-ax",
            ]
            if let rect = bounds(window) { item["bounds"] = rect }
            output.append(item)
        }
    }
    return output
}

private struct ElementReference {
    let pid: pid_t
    let path: [Int]
}

private func parseElementID(_ elementID: String) throws -> ElementReference {
    let parts = elementID.split(separator: ":", maxSplits: 1).map(String.init)
    guard parts.count == 2, let pidInt = Int32(parts[0]), pidInt > 0 else {
        throw NativeError.message("invalid elementId")
    }
    let path = parts[1].split(separator: ".").compactMap { Int($0) }
    guard !path.isEmpty else { throw NativeError.message("invalid element path") }
    return ElementReference(pid: pidInt, path: path)
}

private func resolveElement(pid: pid_t, path: [Int]) throws -> AXUIElement {
    guard !path.isEmpty else { throw NativeError.message("invalid element path") }
    let app = AXUIElementCreateApplication(pid)
    let appWindows = axElements(app, kAXWindowsAttribute)
    guard path[0] >= 0 && path[0] < appWindows.count else { throw NativeError.message("UI element is stale or not found") }
    var element = appWindows[path[0]]
    if path.count > 1 {
        for index in path.dropFirst() {
            let children = axElements(element, kAXChildrenAttribute)
            guard index >= 0 && index < children.count else { throw NativeError.message("UI element is stale or not found") }
            element = children[index]
        }
    }
    return element
}

private func elementSnapshot(_ element: AXUIElement, elementID: String) -> [String: Any] {
    let role = axString(element, kAXRoleAttribute)
    var result: [String: Any] = [
        "elementId": elementID,
        "role": role,
        "name": axString(element, kAXTitleAttribute),
        "description": axString(element, kAXDescriptionAttribute),
        "enabled": axBool(element, kAXEnabledAttribute),
        "engine": "native-ax",
    ]
    if let value = safeValue(element, role: role) { result["value"] = value }
    if let rect = bounds(element) { result["bounds"] = rect }
    return result
}

private func elementRead(pid: pid_t, elementID: String) throws -> [String: Any] {
    let reference = try parseElementID(elementID)
    guard reference.pid == pid else { throw NativeError.message("elementId pid does not match window pid") }
    ObserverRegistry.shared.ensure(pid: pid)
    return elementSnapshot(try resolveElement(pid: pid, path: reference.path), elementID: elementID)
}

private func treeNode(_ element: AXUIElement, pid: pid_t, path: [Int], depth: Int, max: Int, count: inout Int) -> [String: Any]? {
    if count >= max || depth > 8 { return nil }
    count += 1
    let elementID = "\(pid):" + path.map(String.init).joined(separator: ".")
    var item = elementSnapshot(element, elementID: elementID)
    var childNodes: [[String: Any]] = []
    for (index, child) in axElements(element, kAXChildrenAttribute).enumerated() {
        if count >= max { break }
        if let node = treeNode(child, pid: pid, path: path + [index], depth: depth + 1, max: max, count: &count) {
            childNodes.append(node)
        }
    }
    item["children"] = childNodes
    return item
}

private func tree(pid: pid_t, windowIndex: Int, max: Int) throws -> [String: Any] {
    guard AXIsProcessTrusted() else { throw NativeError.message("macOS Accessibility permission is required") }
    let app = AXUIElementCreateApplication(pid)
    let appWindows = axElements(app, kAXWindowsAttribute)
    guard !appWindows.isEmpty else { throw NativeError.message("application process has no accessible windows") }
    ObserverRegistry.shared.ensure(pid: pid)
    var count = 0
    var nodes: [[String: Any]] = []
    if windowIndex >= 0 {
        guard windowIndex < appWindows.count else { throw NativeError.message("window reference is stale") }
        if let node = treeNode(appWindows[windowIndex], pid: pid, path: [windowIndex], depth: 0, max: max, count: &count) { nodes.append(node) }
    } else {
        for (index, window) in appWindows.enumerated() {
            if count >= max { break }
            if let node = treeNode(window, pid: pid, path: [index], depth: 0, max: max, count: &count) { nodes.append(node) }
        }
    }
    return [
        "pid": Int(pid),
        "windowIndex": windowIndex,
        "app": runningAppName(pid: pid),
        "nodes": nodes,
        "truncated": count >= max,
        "engine": "native-ax",
    ]
}

private func normalize(_ value: String) -> String {
    value.trimmingCharacters(in: .whitespacesAndNewlines).lowercased().split(whereSeparator: { $0.isWhitespace }).joined(separator: " ")
}

private func semanticScore(target: String, snapshot: [String: Any]) -> Int {
    let target = normalize(target)
    guard !target.isEmpty else { return 0 }
    var best = 0
    let candidates: [(String, Int, Int)] = [
        (snapshot["name"] as? String ?? "", 100, 70),
        (snapshot["description"] as? String ?? "", 85, 55),
        (snapshot["value"] as? String ?? "", 65, 45),
        (snapshot["role"] as? String ?? "", 35, 20),
    ]
    for (raw, exact, inside) in candidates {
        let text = normalize(raw)
        if text.isEmpty { continue }
        if text == target { best = max(best, exact) }
        else if text.contains(target) || target.contains(text) { best = max(best, inside) }
    }
    let role = normalize(snapshot["role"] as? String ?? "")
    if best > 0 && ["button", "link", "menu", "checkbox", "radio", "textfield", "text field"].contains(where: { role.contains($0) }) {
        best += 10
    }
    if snapshot["enabled"] as? Bool == false { best -= 60 }
    return best
}

private struct SemanticCandidate {
    let element: AXUIElement
    let snapshot: [String: Any]
    let score: Int
}

private func collectCandidates(_ element: AXUIElement, pid: pid_t, path: [Int], depth: Int, max: Int, count: inout Int, target: String, output: inout [SemanticCandidate]) {
    if count >= max || depth > 8 { return }
    count += 1
    let elementID = "\(pid):" + path.map(String.init).joined(separator: ".")
    let snapshot = elementSnapshot(element, elementID: elementID)
    let score = semanticScore(target: target, snapshot: snapshot)
    if score > 0 { output.append(SemanticCandidate(element: element, snapshot: snapshot, score: score)) }
    for (index, child) in axElements(element, kAXChildrenAttribute).enumerated() {
        if count >= max { break }
        collectCandidates(child, pid: pid, path: path + [index], depth: depth + 1, max: max, count: &count, target: target, output: &output)
    }
}

private func semantic(pid: pid_t, windowIndex: Int, operation: String, target: String, text: String, max: Int) throws -> [String: Any] {
    guard AXIsProcessTrusted() else { throw NativeError.message("macOS Accessibility permission is required") }
    let app = AXUIElementCreateApplication(pid)
    let appWindows = axElements(app, kAXWindowsAttribute)
    guard !appWindows.isEmpty else { throw NativeError.message("application process has no accessible windows") }
    ObserverRegistry.shared.ensure(pid: pid)
    var count = 0
    var candidates: [SemanticCandidate] = []
    if windowIndex >= 0 {
        guard windowIndex < appWindows.count else { throw NativeError.message("window reference is stale") }
        collectCandidates(appWindows[windowIndex], pid: pid, path: [windowIndex], depth: 0, max: max, count: &count, target: target, output: &candidates)
    } else {
        for (index, window) in appWindows.enumerated() {
            collectCandidates(window, pid: pid, path: [index], depth: 0, max: max, count: &count, target: target, output: &candidates)
        }
    }
    candidates.sort { $0.score > $1.score }
    guard let best = candidates.first, best.score >= 35 else { throw NativeError.message("no accessible UI element matched \"\(target)\"") }
    if candidates.count > 1 && candidates[1].score >= 35 && best.score - candidates[1].score < 8 {
        throw NativeError.message("ambiguous accessible UI target \"\(target)\"")
    }
    switch operation {
    case "click":
        let error = AXUIElementPerformAction(best.element, kAXPressAction as CFString)
        guard error == .success else { throw NativeError.message("background native AX click failed: \(error.rawValue)") }
    case "type":
        let error = AXUIElementSetAttributeValue(best.element, kAXValueAttribute as CFString, text as CFTypeRef)
        guard error == .success else { throw NativeError.message("background native AX value update failed: \(error.rawValue)") }
    default:
        throw NativeError.message("unsupported semantic operation")
    }
    var resolved = best.snapshot
    resolved["score"] = best.score
    return [
        "operation": operation,
        "background": true,
        "physicalInput": false,
        "engine": "native-ax",
        "resolvedTarget": resolved,
    ]
}

private func semanticBatch(pid: pid_t, windowIndex: Int, steps: [[String: Any]]) throws -> [String: Any] {
    guard !steps.isEmpty && steps.count <= 12 else { throw NativeError.message("semantic batch requires 1-12 steps") }
    let started = Date()
    var results: [[String: Any]] = []
    for (index, step) in steps.enumerated() {
        let operation = (step["operation"] as? String ?? step["action"] as? String ?? "").lowercased()
        let target = step["target"] as? String ?? ""
        let text = step["text"] as? String ?? ""
        guard (operation == "click" || operation == "type") && !target.isEmpty else {
            throw NativeError.message("semantic batch step \(index + 1) is invalid")
        }
        do {
            results.append(try semantic(pid: pid, windowIndex: windowIndex, operation: operation, target: target, text: text, max: 500))
        } catch {
            throw NativeError.message("semantic batch step \(index + 1) failed after \(index) completed step(s): \(error.localizedDescription)")
        }
    }
    return [
        "ok": true,
        "background": true,
        "physicalInput": false,
        "engine": "native-ax-batch",
        "completed": results.count,
        "results": results,
        "durationMs": Int(Date().timeIntervalSince(started) * 1000),
    ]
}

private func axWindowFrame(pid: pid_t, windowIndex: Int) throws -> (title: String, frame: CGRect) {
    let app = AXUIElementCreateApplication(pid)
    let appWindows = axElements(app, kAXWindowsAttribute)
    guard windowIndex >= 0 && windowIndex < appWindows.count else { throw NativeError.message("window reference is stale") }
    let window = appWindows[windowIndex]
    guard let point = axPoint(window, kAXPositionAttribute), let size = axSize(window, kAXSizeAttribute), size.width > 0, size.height > 0 else {
        throw NativeError.message("window has no captureable bounds")
    }
    return (axString(window, kAXTitleAttribute), CGRect(origin: point, size: size))
}

private func frameDistance(_ lhs: CGRect, _ rhs: CGRect) -> Double {
    abs(lhs.minX - rhs.minX) + abs(lhs.minY - rhs.minY) + abs(lhs.width - rhs.width) + abs(lhs.height - rhs.height)
}

private func bestCaptureWindow(_ windows: [SCWindow], pid: pid_t, title: String, frame: CGRect) -> SCWindow? {
    let normalizedTitle = normalize(title)
    return windows
        .filter { $0.owningApplication?.processID == pid && $0.frame.width > 1 && $0.frame.height > 1 }
        .min { left, right in
            let leftTitlePenalty = normalizedTitle.isEmpty || normalize(left.title ?? "") == normalizedTitle ? 0.0 : 10_000.0
            let rightTitlePenalty = normalizedTitle.isEmpty || normalize(right.title ?? "") == normalizedTitle ? 0.0 : 10_000.0
            return frameDistance(left.frame, frame) + leftTitlePenalty < frameDistance(right.frame, frame) + rightTitlePenalty
        }
}

private func pngData(_ image: CGImage) throws -> Data {
    let data = NSMutableData()
    guard let destination = CGImageDestinationCreateWithData(data, UTType.png.identifier as CFString, 1, nil) else {
        throw NativeError.message("could not create PNG encoder")
    }
    CGImageDestinationAddImage(destination, image, nil)
    guard CGImageDestinationFinalize(destination) else { throw NativeError.message("could not encode PNG capture") }
    return data as Data
}

private struct CapturedNativeWindow {
    let image: CGImage
    let frame: CGRect
}

@available(macOS 14.0, *)
private func captureWindowImage(pid: pid_t, windowIndex: Int, maxWidth: Int) async throws -> CapturedNativeWindow {
    guard pid > 0 && windowIndex >= 0 else { throw NativeError.message("native capture requires application pid and windowIndex") }
    let target = try axWindowFrame(pid: pid, windowIndex: windowIndex)
    let content = try await SCShareableContent.excludingDesktopWindows(false, onScreenWindowsOnly: false)
    guard let window = bestCaptureWindow(content.windows, pid: pid, title: target.title, frame: target.frame) else {
        throw NativeError.message("ScreenCaptureKit could not match the AX window")
    }
    let filter = SCContentFilter(desktopIndependentWindow: window)
    let configuration = SCStreamConfiguration()
    let boundedMaxWidth = max(640, min(maxWidth, 1920))
    let scale = min(1.0, Double(boundedMaxWidth) / max(Double(window.frame.width), 1.0))
    configuration.width = max(1, Int(Double(window.frame.width) * scale))
    configuration.height = max(1, Int(Double(window.frame.height) * scale))
    configuration.showsCursor = false
    configuration.ignoreShadowsSingleWindow = true
    let image = try await SCScreenshotManager.captureImage(contentFilter: filter, configuration: configuration)
    return CapturedNativeWindow(image: image, frame: target.frame)
}

@available(macOS 14.0, *)
private func capture(pid: pid_t, windowIndex: Int, maxWidth: Int) async throws -> [String: Any] {
    let captured = try await captureWindowImage(pid: pid, windowIndex: windowIndex, maxWidth: maxWidth)
    let data = try pngData(captured.image)
    return [
        "windowId": "ax:\(pid):\(windowIndex)",
        "mimeType": "image/png",
        "data": data.base64EncodedString(),
        "width": captured.image.width,
        "height": captured.image.height,
        "byteLength": data.count,
        "engine": "screencapturekit",
    ]
}

@available(macOS 14.0, *)
private func vision(pid: pid_t, windowIndex: Int, maxWidth: Int) async throws -> [String: Any] {
    let captured = try await captureWindowImage(pid: pid, windowIndex: windowIndex, maxWidth: maxWidth)
    let request = VNRecognizeTextRequest()
    request.recognitionLevel = .accurate
    request.usesLanguageCorrection = true
    let handler = VNImageRequestHandler(cgImage: captured.image, options: [:])
    try handler.perform([request])

    var nodes: [[String: Any]] = []
    for observation in request.results ?? [] {
        guard nodes.count < 300, let candidate = observation.topCandidates(1).first else { continue }
        let text = candidate.string.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty, candidate.confidence >= 0.25 else { continue }
        let box = observation.boundingBox
        let x = captured.frame.minX + box.minX * captured.frame.width
        let y = captured.frame.minY + (1.0 - box.maxY) * captured.frame.height
        let width = box.width * captured.frame.width
        let height = box.height * captured.frame.height
        let centerX = x + width / 2.0
        let centerY = y + height / 2.0
        nodes.append([
            "elementId": String(format: "vision:%.2f:%.2f", centerX, centerY),
            "windowId": "ax:\(pid):\(windowIndex)",
            "role": "visionText",
            "name": text,
            "description": "Vision OCR text",
            "value": text,
            "enabled": true,
            "source": "vision",
            "confidence": candidate.confidence,
            "bounds": ["x": x, "y": y, "width": width, "height": height],
        ])
    }
    return [
        "nodes": nodes,
        "engine": "native-vision",
        "windowId": "ax:\(pid):\(windowIndex)",
        "imageWidth": captured.image.width,
        "imageHeight": captured.image.height,
    ]
}

private enum NativeError: Error {
    case message(String)
}

extension NativeError: LocalizedError {
    var errorDescription: String? {
        switch self { case .message(let message): return message }
    }
}

private func integer(_ request: [String: Any], _ key: String, fallback: Int = 0) -> Int {
    if let value = request[key] as? Int { return value }
    if let value = request[key] as? NSNumber { return value.intValue }
    return fallback
}

private func handle(_ request: [String: Any]) async throws -> Any {
    let version = integer(request, "version", fallback: protocolVersion)
    guard version == protocolVersion else { throw NativeError.message("unsupported native computer protocol") }
    let op = request["op"] as? String ?? ""
    switch op {
    case "ping":
        return [
            "ready": true,
            "engine": "native-ax",
            "accessibilityTrusted": AXIsProcessTrusted(),
            "sceneEvents": true,
            "protocolVersion": protocolVersion,
        ]
    case "windows":
        return try windows()
    case "tree":
        return try tree(pid: pid_t(integer(request, "pid")), windowIndex: integer(request, "windowIndex", fallback: -1), max: min(max(integer(request, "max", fallback: 500), 1), 1000))
    case "element_read":
        return try elementRead(pid: pid_t(integer(request, "pid")), elementID: request["elementId"] as? String ?? "")
    case "semantic":
        return try semantic(
            pid: pid_t(integer(request, "pid")),
            windowIndex: integer(request, "windowIndex", fallback: -1),
            operation: request["operation"] as? String ?? "",
            target: request["target"] as? String ?? "",
            text: request["text"] as? String ?? "",
            max: min(max(integer(request, "max", fallback: 500), 1), 1000)
        )
    case "semantic_batch":
        return try semanticBatch(
            pid: pid_t(integer(request, "pid")),
            windowIndex: integer(request, "windowIndex", fallback: -1),
            steps: request["steps"] as? [[String: Any]] ?? []
        )
    case "events":
        return SceneEventStore.shared.drain()
    case "capture":
        if #available(macOS 14.0, *) {
            return try await capture(
                pid: pid_t(integer(request, "pid")),
                windowIndex: integer(request, "windowIndex", fallback: -1),
                maxWidth: integer(request, "maxWidth", fallback: 1440)
            )
        }
        throw NativeError.message("ScreenCaptureKit screenshot capture requires macOS 14 or newer")
    case "vision":
        if #available(macOS 14.0, *) {
            return try await vision(
                pid: pid_t(integer(request, "pid")),
                windowIndex: integer(request, "windowIndex", fallback: -1),
                maxWidth: integer(request, "maxWidth", fallback: 1440)
            )
        }
        throw NativeError.message("in-memory native Vision requires macOS 14 or newer")
    default:
        throw NativeError.message("unsupported native computer operation")
    }
}

private func encodeJSON(_ value: Any) throws -> Data {
    try JSONSerialization.data(withJSONObject: value, options: [])
}

private func writeResponse(_ payload: [String: Any]) {
    do {
        var data = try encodeJSON(payload)
        data.append(0x0A)
        FileHandle.standardOutput.write(data)
    } catch {
        let fallback = "{\"ok\":false,\"error\":\"native response encode failed\"}\n"
        FileHandle.standardOutput.write(Data(fallback.utf8))
    }
}

private func capabilities() -> [String: Any] {
    [
        "ready": true,
        "engine": "native-ax",
        "protocolVersion": protocolVersion,
        "nativeAXBackend": true,
        "sceneEvents": true,
        "accessibilityTrusted": AXIsProcessTrusted(),
    ]
}

private func serve() async {
    while let line = readLine() {
        guard let data = line.data(using: .utf8), !data.isEmpty else { continue }
        do {
            guard let request = try JSONSerialization.jsonObject(with: data) as? [String: Any] else {
                throw NativeError.message("invalid native request")
            }
            writeResponse(["ok": true, "result": try await handle(request)])
        } catch {
            writeResponse(["ok": false, "error": error.localizedDescription])
        }
    }
}

private func initializeNativeRuntime() {
    _ = NSApplication.shared
    NSApp.setActivationPolicy(.prohibited)
}

switch CommandLine.arguments.dropFirst().first {
case "--serve":
    initializeNativeRuntime()
    await serve()
case "--capabilities":
    writeResponse(capabilities())
default:
    fputs("usage: codelocal-computer-native --serve|--capabilities\n", stderr)
    exit(2)
}
