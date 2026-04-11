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
    @State private var isShowingLaunchOverlay = true

    var body: some Scene {
        WindowGroup {
            ZStack {
                RootTabView(model: model)

                if isShowingLaunchOverlay {
                    LaunchOverlayView()
                        .transition(.opacity)
                        .allowsHitTesting(false)
                }
            }
            .task {
                await model.start()
                guard isShowingLaunchOverlay else { return }
                try? await Task.sleep(for: .milliseconds(520))
                withAnimation(.easeOut(duration: 0.24)) {
                    isShowingLaunchOverlay = false
                }
            }
            .onChange(of: scenePhase) { _, newPhase in
                if newPhase == .active {
                    Task { await model.handleAppBecameActive() }
                }
            }
        }
    }
}

private struct LaunchOverlayView: View {
    @State private var glossOffset: CGFloat = -240
    @State private var glossOpacity = 0.0

    var body: some View {
        ZStack {
            Color(red: 250/255, green: 248/255, blue: 243/255)
                .ignoresSafeArea()

            ZStack {
                Image("BlackwoodWordmark")
                    .resizable()
                    .scaledToFit()
                    .frame(width: 220, height: 52)

                LinearGradient(
                    colors: [
                        .white.opacity(0.0),
                        .white.opacity(0.45),
                        .white.opacity(0.0),
                    ],
                    startPoint: .top,
                    endPoint: .bottom
                )
                .frame(width: 54, height: 72)
                .rotationEffect(.degrees(18))
                .offset(x: glossOffset)
                .opacity(glossOpacity)
                .blendMode(.screen)
                .mask {
                    Image("BlackwoodWordmark")
                        .resizable()
                        .scaledToFit()
                        .frame(width: 220, height: 52)
                }
            }
        }
        .task {
            glossOpacity = 1
            withAnimation(.easeInOut(duration: 0.48)) {
                glossOffset = 240
            }
            try? await Task.sleep(for: .milliseconds(360))
            withAnimation(.easeOut(duration: 0.18)) {
                glossOpacity = 0
            }
        }
    }
}
