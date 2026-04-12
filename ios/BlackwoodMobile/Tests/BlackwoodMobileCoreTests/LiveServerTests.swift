import Foundation
import Testing
@testable import BlackwoodMobileCore

private func liveBaseURL() -> URL {
    let raw = ProcessInfo.processInfo.environment["BLACKWOOD_E2E_BASE_URL"] ?? "http://127.0.0.1:18080"
    guard let url = URL(string: raw) else {
        fatalError("Invalid BLACKWOOD_E2E_BASE_URL: \(raw)")
    }
    return url
}

private func testDate(offsetDays: Int) -> String {
    let calendar = Calendar(identifier: .gregorian)
    let date = calendar.date(byAdding: .day, value: offsetDays, to: Date()) ?? Date()
    let formatter = DateFormatter()
    formatter.calendar = calendar
    formatter.dateFormat = "yyyy-MM-dd"
    return formatter.string(from: date)
}

@Test
func liveWireProtocolRoundTrip() async throws {
    let client = BlackwoodAPIClient(baseURL: liveBaseURL())
    let emptyDate = testDate(offsetDays: 7)
    let populatedDate = testDate(offsetDays: 8)

    let health = try await client.checkHealth()
    #expect(health.status == "ok")

    let emptyNote = try await client.fetchDailyNote(date: emptyDate)
    #expect(!emptyNote.id.isEmpty)
    #expect(emptyNote.entries.isEmpty)
    #expect(emptyNote.content == "")

    let date = populatedDate
    let template = "# Summary\n\nIntegration test note.\n\n# Notes\n\n# Links\n"

    let updated = try await client.updateDailyNoteContent(date: date, content: template, baseRevision: "")
    #expect(updated.content.contains("Integration test note."))

    let tempDir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
    try FileManager.default.createDirectory(at: tempDir, withIntermediateDirectories: true)
    let audioURL = tempDir.appendingPathComponent("integration-clip.m4a")
    try Data("fake-m4a-audio".utf8).write(to: audioURL)

    _ = try await client.createAudioEntry(
        upload: PendingEntryUpload(
            date: date,
            localFilePath: audioURL.path,
            filename: "integration-clip.m4a",
            contentType: "audio/x-m4a",
            duration: 2
        )
    )

    let fetched = try await client.fetchDailyNote(date: date)
    #expect(fetched.content.contains("Integration test note."))
    #expect(fetched.content.contains("Voice memo"))
    #expect(fetched.entries.count >= 1)
}
