import Foundation
import AppKit
import ApplicationServices
import ScreenCaptureKit
import Vision
import ImageIO
import UniformTypeIdentifiers
import Darwin

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

private func sensitiveFieldMetadata(role: String, name: String, description: String) -> Bool {
    let haystack = "\(role) \(name) \(description)".lowercased()
    let markers = [
        "secure", "password", "passcode", "otp", "2fa", "verification code",
        "one-time code", "one time code", "authenticator code", "security code",
        "api key", "private key", "seed phrase", "secret key", "credential",
    ]
    return markers.contains { haystack.contains($0) }
}

private func safeValue(_ element: AXUIElement, role: String, name: String, description: String) -> Any? {
    if sensitiveFieldMetadata(role: role, name: name, description: description) { return nil }
    guard let value = axAttribute(element, kAXValueAttribute) else { return nil }
    if let text = value as? String { return text }
    if let number = value as? NSNumber { return number }
    return nil
}

private func runningAppName(pid: pid_t) -> String {
    NSRunningApplication(processIdentifier: pid)?.localizedName ?? ""
}

private func windows() throws -> [[String: Any]] {
    try requireNativeControl(observation: true)
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
    let name = axString(element, kAXTitleAttribute)
    let description = axString(element, kAXDescriptionAttribute)
    var result: [String: Any] = [
        "elementId": elementID,
        "role": role,
        "name": name,
        "description": description,
        "enabled": axBool(element, kAXEnabledAttribute),
        "engine": "native-ax",
    ]
    if let value = safeValue(element, role: role, name: name, description: description) { result["value"] = value }
    if let rect = bounds(element) { result["bounds"] = rect }
    return result
}

private func elementRead(pid: pid_t, elementID: String) throws -> [String: Any] {
    try requireNativeControl(observation: true)
    let reference = try parseElementID(elementID)
    guard reference.pid == pid else { throw NativeError.message("elementId pid does not match window pid") }
    ObserverRegistry.shared.ensure(pid: pid)
    return elementSnapshot(try resolveElement(pid: pid, path: reference.path), elementID: elementID)
}

private func elementAction(pid: pid_t, elementID: String, operation: String, text: String) throws -> [String: Any] {
    try requireNativeControl(mutation: true)
    let reference = try parseElementID(elementID)
    guard reference.pid == pid else { throw NativeError.message("elementId pid does not match window pid") }
    ObserverRegistry.shared.ensure(pid: pid)
    let element = try resolveElement(pid: pid, path: reference.path)
    guard axBool(element, kAXEnabledAttribute) else { throw NativeError.message("UI element is disabled") }
    switch operation {
    case "click":
        let error = AXUIElementPerformAction(element, kAXPressAction as CFString)
        guard error == .success else { throw NativeError.message("background native AX click failed: \(error.rawValue)") }
    case "type":
        let error = AXUIElementSetAttributeValue(element, kAXValueAttribute as CFString, text as CFTypeRef)
        guard error == .success else { throw NativeError.message("background native AX value update failed: \(error.rawValue)") }
    default:
        throw NativeError.message("unsupported exact element operation")
    }
    var resolved = elementSnapshot(element, elementID: elementID)
    if operation == "type" { resolved.removeValue(forKey: "value") }
    return [
        "operation": operation,
        "background": true,
        "physicalInput": false,
        "engine": "native-ax-element",
        "resolvedTarget": resolved,
    ]
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
    try requireNativeControl(observation: true)
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
    try requireNativeControl(mutation: true)
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
    if operation == "type" { resolved.removeValue(forKey: "value") }
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

private func encodedImageData(_ image: CGImage, type: UTType, properties: [CFString: Any]? = nil) throws -> Data {
    let data = NSMutableData()
    guard let destination = CGImageDestinationCreateWithData(data, type.identifier as CFString, 1, nil) else {
        throw NativeError.message("could not create \(type.identifier) encoder")
    }
    CGImageDestinationAddImage(destination, image, properties as CFDictionary?)
    guard CGImageDestinationFinalize(destination) else {
        throw NativeError.message("could not encode \(type.identifier) capture")
    }
    return data as Data
}

private struct EncodedNativeImage {
    let data: Data
    let mimeType: String
    let encoding: String
}

private func encodeNativeScreenshot(_ image: CGImage) throws -> EncodedNativeImage {
    let png = try encodedImageData(image, type: .png)
    guard png.count >= 64 * 1024 else {
        return EncodedNativeImage(data: png, mimeType: "image/png", encoding: "png")
    }
    let webP: Data
    do {
        webP = try encodedImageData(
            image,
            type: .webP,
            properties: [kCGImageDestinationLossyCompressionQuality: 0.92]
        )
    } catch {
        return EncodedNativeImage(data: png, mimeType: "image/png", encoding: "png")
    }
    if webP.count * 100 <= png.count * 80 {
        return EncodedNativeImage(data: webP, mimeType: "image/webp", encoding: "webp")
    }
    return EncodedNativeImage(data: png, mimeType: "image/png", encoding: "png")
}

private struct CapturedNativeWindow {
    let image: CGImage
    let frame: CGRect
}

@available(macOS 14.0, *)
private func captureWindowImage(pid: pid_t, windowIndex: Int, maxWidth: Int) async throws -> CapturedNativeWindow {
    try requireNativeControl(observation: true)
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
    let encoded = try encodeNativeScreenshot(captured.image)
    return [
        "windowId": "ax:\(pid):\(windowIndex)",
        "mimeType": encoded.mimeType,
        "data": encoded.data.base64EncodedString(),
        "width": captured.image.width,
        "height": captured.image.height,
        "byteLength": encoded.data.count,
        "encoding": encoded.encoding,
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

private let controlStateFile = ProcessInfo.processInfo.environment["CODELOCAL_COMPUTER_CONTROL_STATE_FILE"] ?? ""
private let activityStateFile = ProcessInfo.processInfo.environment["CODELOCAL_COMPUTER_ACTIVITY_STATE_FILE"] ?? ""

private func atomicWrite(_ data: Data, to path: String) throws {
    guard !path.isEmpty else { return }
    let url = URL(fileURLWithPath: path)
    try FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
    try data.write(to: url, options: .atomic)
    try? FileManager.default.setAttributes([.posixPermissions: NSNumber(value: Int16(0o600))], ofItemAtPath: path)
}

private func readControlState(at path: String) -> String {
    guard !path.isEmpty,
          let data = FileManager.default.contents(atPath: path),
          let raw = String(data: data, encoding: .utf8) else { return "active" }
    switch raw.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() {
    case "paused": return "paused"
    case "stopped": return "stopped"
    default: return "active"
    }
}

private func requireNativeControl(observation: Bool = false, mutation: Bool = false) throws {
    let state = readControlState(at: controlStateFile)
    if state == "stopped" && (observation || mutation) {
        throw NativeError.message("Computer Use is stopped from the CodeLocal menu bar")
    }
    if state == "paused" && mutation {
        throw NativeError.message("Computer Use control is paused from the CodeLocal menu bar")
    }
}

private func activityAppName(windowID: String) -> String {
    let parts = windowID.split(separator: ":")
    guard parts.count == 3, parts[0] == "ax", let pid = Int32(parts[1]) else { return "" }
    return runningAppName(pid: pid)
}

@discardableResult
private func writeActivityState(mode: String, windowID: String, detail: String) throws -> [String: Any] {
    let normalizedMode: String
    switch mode.lowercased() {
    case "background", "viewing", "foreground", "idle": normalizedMode = mode.lowercased()
    default: normalizedMode = "idle"
    }
    let payload: [String: Any] = [
        "mode": normalizedMode,
        "windowId": windowID,
        "app": activityAppName(windowID: windowID),
        "detail": detail,
        "timestampMs": Int(Date().timeIntervalSince1970 * 1000),
    ]
    if !activityStateFile.isEmpty {
        try atomicWrite(try JSONSerialization.data(withJSONObject: payload), to: activityStateFile)
    }
    return payload
}

private final class IndicatorProcessOwner {
    static let shared = IndicatorProcessOwner()
    private var process: Process?

    func startIfEnabled() {
        guard ProcessInfo.processInfo.environment["CODELOCAL_COMPUTER_ACTIVITY_INDICATOR"] == "1",
              !controlStateFile.isEmpty, !activityStateFile.isEmpty else { return }
        let process = Process()
        process.executableURL = URL(fileURLWithPath: CommandLine.arguments[0])
        process.arguments = ["--indicator", controlStateFile, activityStateFile, String(getpid())]
        process.standardOutput = FileHandle.nullDevice
        process.standardError = FileHandle.nullDevice
        do {
            try process.run()
            self.process = process
        } catch {
            self.process = nil
        }
    }

    func stop() {
        guard let process else { return }
        if process.isRunning { process.terminate() }
        self.process = nil
    }
}

@MainActor
private final class ComputerActivityStatusItem: NSObject {
    private let controlPath: String
    private let activityPath: String
    private let parentPID: pid_t
    private let statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
    private let stateItem = NSMenuItem(title: "Idle", action: nil, keyEquivalent: "")
    private let appItem = NSMenuItem(title: "No active app", action: nil, keyEquivalent: "")
    private let pauseItem = NSMenuItem(title: "Pause Computer Control", action: #selector(togglePause), keyEquivalent: "")
    private let stopItem = NSMenuItem(title: "Stop Computer Control", action: #selector(stopControl), keyEquivalent: "")
    private var timer: Timer?

    init(controlPath: String, activityPath: String, parentPID: pid_t) {
        self.controlPath = controlPath
        self.activityPath = activityPath
        self.parentPID = parentPID
        super.init()
    }

    func start() {
        NSApp.setActivationPolicy(.accessory)
        statusItem.button?.title = "○ CodeLocal"
        statusItem.button?.toolTip = "CodeLocal Computer Use · Idle"

        stateItem.isEnabled = false
        appItem.isEnabled = false
        pauseItem.target = self
        stopItem.target = self

        let menu = NSMenu()
        let title = NSMenuItem(title: "CodeLocal Computer Use", action: nil, keyEquivalent: "")
        title.isEnabled = false
        menu.addItem(title)
        menu.addItem(stateItem)
        menu.addItem(appItem)
        menu.addItem(.separator())
        menu.addItem(pauseItem)
        menu.addItem(stopItem)
        statusItem.menu = menu

        refresh()
        timer = Timer.scheduledTimer(timeInterval: 0.25, target: self, selector: #selector(timerFired), userInfo: nil, repeats: true)
    }

    @objc private func timerFired() {
        refresh()
    }

    private func controlState() -> String { readControlState(at: controlPath) }

    private func activity() -> [String: Any] {
        guard let data = FileManager.default.contents(atPath: activityPath),
              let value = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else { return [:] }
        return value
    }

    private func parentAlive() -> Bool {
        if kill(parentPID, 0) == 0 { return true }
        return errno != ESRCH
    }

    private func render(mode: String, state: String, app: String, detail: String) {
        let title: String
        let label: String
        if state == "stopped" {
            title = "■ CodeLocal"
            label = "Stopped"
        } else if state == "paused" {
            title = "Ⅱ CodeLocal"
            label = "Paused"
        } else {
            switch mode {
            case "viewing":
                title = "◉ CodeLocal"
                label = "Viewing Screen"
            case "foreground":
                title = "⚠︎ CodeLocal"
                label = "Foreground Control"
            case "background":
                title = "● CodeLocal"
                label = detail.isEmpty ? "Background Control" : detail
            default:
                title = "○ CodeLocal"
                label = "Idle"
            }
        }
        statusItem.button?.title = title
        statusItem.button?.toolTip = "CodeLocal Computer Use · \(label)"
        stateItem.title = label
        appItem.title = app.isEmpty ? "No active app" : "App: \(app)"
        pauseItem.title = state == "active" ? "Pause Computer Control" : "Resume Computer Control"
        stopItem.isEnabled = state != "stopped"
    }

    private func refresh() {
        guard parentAlive() else {
            NSApp.terminate(nil)
            return
        }
        let state = controlState()
        let activity = activity()
        var mode = activity["mode"] as? String ?? "idle"
        let app = activity["app"] as? String ?? ""
        let detail = activity["detail"] as? String ?? ""
        let timestamp = activity["timestampMs"] as? Int ?? 0
        if timestamp > 0 && Int(Date().timeIntervalSince1970 * 1000) - timestamp > 3000 {
            mode = "idle"
        }
        render(mode: mode, state: state, app: app, detail: detail)
    }

    private func writeControl(_ state: String) {
        try? atomicWrite(Data((state + "\n").utf8), to: controlPath)
        refresh()
    }

    @objc private func togglePause() {
        writeControl(controlState() == "active" ? "paused" : "active")
    }

    @objc private func stopControl() {
        writeControl("stopped")
    }
}

@MainActor
private func runIndicator(controlPath: String, activityPath: String, parentPID: pid_t) {
    _ = NSApplication.shared
    let controller = ComputerActivityStatusItem(controlPath: controlPath, activityPath: activityPath, parentPID: parentPID)
    controller.start()
    NSApp.run()
    _ = controller
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
    case "element_action":
        return try elementAction(
            pid: pid_t(integer(request, "pid")),
            elementID: request["elementId"] as? String ?? "",
            operation: request["operation"] as? String ?? "",
            text: request["text"] as? String ?? ""
        )
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
    case "activity":
        return try writeActivityState(
            mode: request["mode"] as? String ?? "idle",
            windowID: request["windowId"] as? String ?? "",
            detail: request["detail"] as? String ?? ""
        )
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
    IndicatorProcessOwner.shared.startIfEnabled()
    _ = try? writeActivityState(mode: "idle", windowID: "", detail: "")
    defer { IndicatorProcessOwner.shared.stop() }
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
case "--indicator":
    let args = Array(CommandLine.arguments.dropFirst(2))
    guard args.count == 3, let parentPID = Int32(args[2]), parentPID > 0 else {
        fputs("usage: codelocal-computer-native --indicator <control-state> <activity-state> <parent-pid>\n", stderr)
        exit(2)
    }
    runIndicator(controlPath: args[0], activityPath: args[1], parentPID: parentPID)
default:
    fputs("usage: codelocal-computer-native --serve|--capabilities|--indicator\n", stderr)
    exit(2)
}
