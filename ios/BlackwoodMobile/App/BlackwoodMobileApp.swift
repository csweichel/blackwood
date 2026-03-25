import AppIntents
import SwiftUI

enum ShortcutKeys {
    static let startRecording = "blackwood.shortcut.startRecording"
}

struct StartRecordingIntent: AppIntent {
    static let title: LocalizedStringResource = "Start Blackwood Recording"
    static let description = IntentDescription("Open Blackwood directly into voice recording.")
    static let openAppWhenRun = true

    func perform() async throws -> some IntentResult {
        UserDefaults.standard.set(true, forKey: ShortcutKeys.startRecording)
        return .result()
    }
}

struct BlackwoodShortcutsProvider: AppShortcutsProvider {
    static var appShortcuts: [AppShortcut] {
        AppShortcut(
            intent: StartRecordingIntent(),
            phrases: [
                "Start recording in \(.applicationName)",
                "Record a note in \(.applicationName)",
            ],
            shortTitle: "Start Recording",
            systemImageName: "mic.circle.fill"
        )
    }
}

@main
struct BlackwoodMobileApp: App {
    @Environment(\.scenePhase) private var scenePhase
    @StateObject private var model = AppModel()

    var body: some Scene {
        WindowGroup {
            RootTabView(model: model)
                .task {
                    await model.start()
                }
                .onChange(of: scenePhase) { _, newPhase in
                    if newPhase == .active {
                        Task { await model.handleAppBecameActive() }
                    }
                }
        }
    }
}
