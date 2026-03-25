import AVFoundation
import Foundation

@MainActor
final class AudioRecorderController: NSObject, ObservableObject, AVAudioRecorderDelegate {
    enum State: Equatable {
        case idle
        case preparing
        case recording
        case failed(String)
    }

    @Published var state: State = .idle
    @Published var duration: TimeInterval = 0
    var autoStartOnAppear = false
    var onFinishedRecording: ((URL, TimeInterval) -> Void)?

    private var recorder: AVAudioRecorder?
    private var timer: Timer?
    private var startedAt: Date?
    private var latestMeasuredDuration: TimeInterval = 0

    func prepareIfNeeded() async {
        guard autoStartOnAppear else { return }
        autoStartOnAppear = false
        await startRecording()
    }

    func startRecording() async {
        state = .preparing
        let session = AVAudioSession.sharedInstance()
        let granted = await requestPermission(session: session)
        guard granted else {
            state = .failed("Microphone permission is required before the shortcut can auto-start recording.")
            return
        }

        do {
            try session.setCategory(.playAndRecord, mode: .default, options: [.defaultToSpeaker])
            try session.setActive(true)

            let fileURL = try Self.makeRecordingURL()
            let settings: [String: Any] = [
                AVFormatIDKey: Int(kAudioFormatMPEG4AAC),
                AVSampleRateKey: 44_100,
                AVNumberOfChannelsKey: 1,
                AVEncoderAudioQualityKey: AVAudioQuality.high.rawValue,
            ]

            let recorder = try AVAudioRecorder(url: fileURL, settings: settings)
            recorder.delegate = self
            recorder.record()
            self.recorder = recorder
            self.startedAt = Date()
            self.duration = 0
            self.latestMeasuredDuration = 0
            self.state = .recording

            timer?.invalidate()
            timer = Timer.scheduledTimer(withTimeInterval: 0.25, repeats: true) { [weak self] _ in
                guard let self, let startedAt else { return }
                let elapsed = Date().timeIntervalSince(startedAt)
                self.duration = elapsed
                self.latestMeasuredDuration = elapsed
            }
        } catch {
            state = .failed(error.localizedDescription)
        }
    }

    func stopRecording() {
        timer?.invalidate()
        timer = nil
        recorder?.stop()
    }

    func dismissError() {
        state = .idle
    }

    func audioRecorderDidFinishRecording(_ recorder: AVAudioRecorder, successfully flag: Bool) {
        defer {
            self.recorder = nil
            self.startedAt = nil
            self.duration = 0
            self.state = .idle
        }
        guard flag else {
            state = .failed("Recording failed to save.")
            return
        }
        let finalDuration = max(latestMeasuredDuration, recorder.currentTime)
        onFinishedRecording?(recorder.url, finalDuration)
    }

    private func requestPermission(session: AVAudioSession) async -> Bool {
        await withCheckedContinuation { continuation in
            session.requestRecordPermission { granted in
                continuation.resume(returning: granted)
            }
        }
    }

    private static func makeRecordingURL() throws -> URL {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? URL(fileURLWithPath: NSTemporaryDirectory(), isDirectory: true)
        let recordings = base.appendingPathComponent("BlackwoodMobile/Recordings", isDirectory: true)
        try FileManager.default.createDirectory(at: recordings, withIntermediateDirectories: true, attributes: nil)
        return recordings.appendingPathComponent("recording-\(UUID().uuidString).m4a")
    }
}
