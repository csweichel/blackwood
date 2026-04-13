import Foundation

public protocol BlackwoodRemote: Sendable {
    func fetchDailyNote(date: String) async throws -> APIDailyNote
    func updateDailyNoteContent(date: String, content: String, baseRevision: String) async throws -> APIDailyNote
    func fetchSubpage(date: String, name: String) async throws -> APISubpage
    func updateSubpageContent(date: String, name: String, content: String, baseRevision: String) async throws -> APISubpage
    func createAudioEntry(upload: PendingEntryUpload) async throws -> APIEntry
    func search(query: String, limit: Int) async throws -> [SearchResult]
    func checkHealth() async throws -> HealthCheckResponse
    func makeChangeStream() -> AsyncThrowingStream<APIChangeEvent, Error>
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

    public func updateDailyNoteContent(date: String, content: String, baseRevision: String) async throws -> APIDailyNote {
        try await rpc(
            path: "/blackwood.v1.DailyNotesService/UpdateDailyNoteContent",
            request: ["date": date, "content": content, "baseRevision": baseRevision]
        )
    }

    public func fetchSubpage(date: String, name: String) async throws -> APISubpage {
        try await rpc(
            path: "/blackwood.v1.DailyNotesService/GetSubpage",
            request: ["date": date, "name": name]
        )
    }

    public func updateSubpageContent(date: String, name: String, content: String, baseRevision: String) async throws -> APISubpage {
        try await rpc(
            path: "/blackwood.v1.DailyNotesService/UpdateSubpageContent",
            request: ["date": date, "name": name, "content": content, "baseRevision": baseRevision]
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

    public func makeChangeStream() -> AsyncThrowingStream<APIChangeEvent, Error> {
        AsyncThrowingStream { continuation in
            let task = Task {
                do {
                    var urlRequest = URLRequest(url: baseURL.appending(path: "/blackwood.v1.DailyNotesService/StreamChanges"))
                    urlRequest.httpMethod = "POST"
                    urlRequest.setValue("application/connect+json", forHTTPHeaderField: "Content-Type")
                    urlRequest.setValue("1", forHTTPHeaderField: "Connect-Protocol-Version")
                    urlRequest.httpBody = envelopeRequest([:])

                    let (bytes, response) = try await session.bytes(for: urlRequest)
                    try validate(response: response, data: Data())

                    var iterator = bytes.makeAsyncIterator()
                    var buffer = Data()
                    let decoder = JSONDecoder()
                    decoder.keyDecodingStrategy = .convertFromSnakeCase

                    while let byte = try await iterator.next() {
                        buffer.append(byte)

                        while buffer.count >= 5 {
                            let flags = buffer[0]
                            let lengthData = buffer[1..<5]
                            let length = lengthData.reduce(UInt32(0)) { ($0 << 8) | UInt32($1) }
                            let frameSize = 5 + Int(length)
                            guard buffer.count >= frameSize else { break }

                            let payload = buffer[5..<frameSize]
                            buffer.removeFirst(frameSize)

                            if (flags & 0x01) != 0 {
                                throw SyncFailure(message: "Compressed stream frames are not supported.", disposition: .terminal)
                            }

                            if (flags & 0x02) != 0 {
                                let streamError = parsedErrorPayload(from: Data(payload))
                                if let message = streamError.message {
                                    throw SyncFailure(message: message, disposition: .retryable, code: streamError.code)
                                }
                                continue
                            }

                            let envelope = try decoder.decode(StreamEnvelope<APIChangeEvent>.self, from: Data(payload))
                            continuation.yield(envelope.result ?? envelope.directResult)
                        }
                    }

                    continuation.finish()
                } catch {
                    continuation.finish(throwing: error)
                }
            }

            continuation.onTermination = { _ in
                task.cancel()
            }
        }
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
            let payload = parsedErrorPayload(from: data)
            let text = payload.message ?? String(data: data, encoding: .utf8) ?? "Request failed"
            if http.statusCode == 401 {
                let kind: AuthChallengeKind = payload.code == "setup_required" ? .setupRequired : .unauthorized
                throw AuthChallenge(kind: kind, message: text)
            }
            let disposition: SyncFailureDisposition = http.statusCode >= 500 ? .retryable : .terminal
            throw SyncFailure(message: text, disposition: disposition, code: payload.code)
        }
    }

    private func parsedErrorPayload(from data: Data) -> (code: String?, message: String?) {
        guard
            let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any]
        else {
            return (nil, nil)
        }

        let code = json["code"] as? String

        for key in ["message", "error", "msg"] {
            if let value = json[key] as? String, !value.isEmpty {
                return (code, value)
            }
        }

        if let error = json["error"] as? [String: Any] {
            for key in ["message", "msg"] {
                if let value = error[key] as? String, !value.isEmpty {
                    return (code, value)
                }
            }
        }

        return (code, nil)
    }

    private func envelopeRequest(_ payload: [String: Any]) -> Data {
        let json = (try? JSONSerialization.data(withJSONObject: payload)) ?? Data("{}".utf8)
        var framed = Data([0, 0, 0, 0, 0])
        let length = UInt32(json.count).bigEndian
        withUnsafeBytes(of: length) { framed.replaceSubrange(1..<5, with: $0) }
        framed.append(json)
        return framed
    }
}

private struct StreamEnvelope<Result: Decodable>: Decodable {
    let result: Result?
    let directResult: Result

    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        if let wrapped = try? container.decode(ResultWrapper<Result>.self), let result = wrapped.result {
            self.result = result
            self.directResult = result
            return
        }
        let direct = try container.decode(Result.self)
        self.result = nil
        self.directResult = direct
    }
}

private struct ResultWrapper<Result: Decodable>: Decodable {
    let result: Result?
}
