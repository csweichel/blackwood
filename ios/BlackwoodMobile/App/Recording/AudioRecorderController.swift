import AVFoundation
import Foundation

@MainActor
final class AudioRecorderController: NSObject, ObservableObject, @preconcurrency AVAudioRecorderDelegate {
    enum State: Equatable {
        case idle
        case preparing
        case recording
        case processing
        case completed(TimeInterval)
        case failed(String)
    }

    @Published var state: State = .idle
    @Published var duration: TimeInterval = 0
    @Published var levels: [CGFloat] = Array(repeating: 0.12, count: 24)
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

    func reset() {
        timer?.invalidate()
        timer = nil
        recorder = nil
        startedAt = nil
        latestMeasuredDuration = 0
        duration = 0
        levels = Array(repeating: 0.12, count: 24)
        state = .idle
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
                AVEncoderBitRateKey: 128_000,
                AVEncoderAudioQualityKey: AVAudioQuality.high.rawValue,
            ]

            let recorder = try AVAudioRecorder(url: fileURL, settings: settings)
            recorder.delegate = self
            recorder.isMeteringEnabled = true
            recorder.prepareToRecord()
            recorder.record()
            let recordingStart = Date()
            self.recorder = recorder
            self.startedAt = recordingStart
            self.duration = 0
            self.latestMeasuredDuration = 0
            self.levels = Array(repeating: 0.12, count: 24)
            self.state = .recording

            timer?.invalidate()
            timer = Timer.scheduledTimer(withTimeInterval: 0.25, repeats: true) { [weak self, recorder, recordingStart] _ in
                Task { @MainActor [weak self] in
                    guard let self else { return }
                    let elapsed = Date().timeIntervalSince(recordingStart)
                    recorder.updateMeters()
                    self.duration = elapsed
                    self.latestMeasuredDuration = elapsed
                    self.levels.removeFirst()
                    self.levels.append(Self.normalizedLevel(from: recorder.averagePower(forChannel: 0)))
                }
            }
        } catch {
            state = .failed(error.localizedDescription)
        }
    }

    func stopRecording() {
        timer?.invalidate()
        timer = nil
        state = .processing
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
        }
        guard flag else {
            state = .failed("Recording failed to save.")
            return
        }
        let finalDuration = max(latestMeasuredDuration, recorder.currentTime)
        state = .completed(finalDuration)
        onFinishedRecording?(recorder.url, finalDuration)
    }

    private func requestPermission(session: AVAudioSession) async -> Bool {
        await withCheckedContinuation { continuation in
            if #available(iOS 17.0, *) {
                AVAudioApplication.requestRecordPermission { granted in
                    continuation.resume(returning: granted)
                }
            } else {
                session.requestRecordPermission { granted in
                    continuation.resume(returning: granted)
                }
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

    private static func normalizedLevel(from averagePower: Float) -> CGFloat {
        let floor: Float = -50
        guard averagePower.isFinite, averagePower > floor else { return 0.12 }
        let normalized = (averagePower - floor) / abs(floor)
        return max(0.12, CGFloat(normalized))
    }
}
