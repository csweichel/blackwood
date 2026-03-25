import Foundation

public protocol BlackwoodRemote: Sendable {
    func fetchDailyNote(date: String) async throws -> APIDailyNote
    func updateDailyNoteContent(date: String, content: String) async throws -> APIDailyNote
    func createAudioEntry(upload: PendingEntryUpload) async throws -> APIEntry
    func search(query: String, limit: Int) async throws -> [SearchResult]
    func checkHealth() async throws -> HealthCheckResponse
}

public struct HealthCheckResponse: Codable, Equatable, Sendable {
    public let status: String
    public let version: String
}

public struct BlackwoodAPIClient: BlackwoodRemote, Sendable {
    public let baseURL: URL
    private let session: URLSession

    public init(baseURL: URL, session: URLSession = .shared) {
        self.baseURL = baseURL
        self.session = session
    }

    public func fetchDailyNote(date: String) async throws -> APIDailyNote {
        try await rpc(
            path: "/blackwood.v1.DailyNotesService/GetDailyNote",
            request: ["date": date]
        )
    }

    public func updateDailyNoteContent(date: String, content: String) async throws -> APIDailyNote {
        try await rpc(
            path: "/blackwood.v1.DailyNotesService/UpdateDailyNoteContent",
            request: ["date": date, "content": content]
        )
    }

    public func createAudioEntry(upload: PendingEntryUpload) async throws -> APIEntry {
        let fileURL = URL(fileURLWithPath: upload.localFilePath)
        guard FileManager.default.fileExists(atPath: fileURL.path) else {
            throw SyncFailure(
                message: "This recording is no longer stored on the device. Remove it from the queue and record again.",
                disposition: .terminal
            )
        }

        let data: Data
        do {
            data = try Data(contentsOf: fileURL)
        } catch {
            throw SyncFailure(
                message: "Blackwood couldn't reopen this recording file. Remove it from the queue and record again.",
                disposition: .terminal
            )
        }

        guard !data.isEmpty else {
            throw SyncFailure(
                message: "This recording file is empty, so it can't be uploaded.",
                disposition: .terminal
            )
        }

        let body: [String: Any] = [
            "date": upload.date,
            "type": upload.type,
            "content": upload.content,
            "source": upload.source,
            "clientRequestId": upload.clientRequestId,
            "attachmentData": [data.base64EncodedString()],
            "attachmentFilenames": [upload.filename],
            "attachmentContentTypes": [upload.contentType],
        ]
        let url = baseURL.appending(path: "/blackwood.v1.DailyNotesService/CreateEntry")
        var urlRequest = URLRequest(url: url)
        urlRequest.httpMethod = "POST"
        urlRequest.setValue("application/json", forHTTPHeaderField: "Content-Type")
        urlRequest.httpBody = try JSONSerialization.data(withJSONObject: body)
        let (responseData, response) = try await session.data(for: urlRequest)
        try validate(response: response, data: responseData)

        // Sync only needs success/failure. The Connect JSON response shape can evolve,
        // so return a lightweight placeholder after validation instead of requiring a
        // full decode for the upload path.
        return APIEntry(
            id: upload.clientRequestId,
            dailyNoteId: "",
            type: upload.type,
            content: upload.content,
            rawContent: upload.content,
            source: upload.source,
            metadata: "",
            attachments: [],
            createdAt: ISO8601DateFormatter().string(from: upload.createdAt),
            updatedAt: ISO8601DateFormatter().string(from: Date())
        )
    }

    public func search(query: String, limit: Int = 20) async throws -> [SearchResult] {
        var components = URLComponents(url: baseURL.appending(path: "/api/search"), resolvingAgainstBaseURL: false)
        components?.queryItems = [
            URLQueryItem(name: "q", value: query),
            URLQueryItem(name: "limit", value: String(limit)),
        ]
        guard let url = components?.url else {
            throw SyncFailure(message: "Invalid search URL", disposition: .terminal)
        }

        let (data, response) = try await session.data(from: url)
        try validate(response: response, data: data)
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        return try decoder.decode(SearchResponse.self, from: data).results
    }

    public func checkHealth() async throws -> HealthCheckResponse {
        try await rpc(
            path: "/blackwood.v1.HealthService/Check",
            request: [:]
        )
    }

    private func rpc<Response: Decodable>(path: String, request: [String: Any]) async throws -> Response {
        let url = baseURL.appending(path: path)
        var urlRequest = URLRequest(url: url)
        urlRequest.httpMethod = "POST"
        urlRequest.setValue("application/json", forHTTPHeaderField: "Content-Type")
        urlRequest.httpBody = try JSONSerialization.data(withJSONObject: request)
        let (data, response) = try await session.data(for: urlRequest)
        try validate(response: response, data: data)
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        return try decoder.decode(Response.self, from: data)
    }

    private func validate(response: URLResponse, data: Data) throws {
        guard let http = response as? HTTPURLResponse else {
            throw SyncFailure(message: "Invalid server response", disposition: .retryable)
        }
        guard (200..<300).contains(http.statusCode) else {
            let text = parsedErrorMessage(from: data) ?? String(data: data, encoding: .utf8) ?? "Request failed"
            let disposition: SyncFailureDisposition = http.statusCode >= 500 ? .retryable : .terminal
            throw SyncFailure(message: text, disposition: disposition)
        }
    }

    private func parsedErrorMessage(from data: Data) -> String? {
        guard
            let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any]
        else {
            return nil
        }

        for key in ["message", "error", "msg"] {
            if let value = json[key] as? String, !value.isEmpty {
                return value
            }
        }

        if let error = json["error"] as? [String: Any] {
            for key in ["message", "msg"] {
                if let value = error[key] as? String, !value.isEmpty {
                    return value
                }
            }
        }

        return nil
    }
}
