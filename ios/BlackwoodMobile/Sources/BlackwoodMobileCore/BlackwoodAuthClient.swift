import Foundation

public struct AuthStatusResponse: Codable, Equatable, Sendable {
    public let authenticated: Bool
    public let setupRequired: Bool
}

public struct AuthSetupInfo: Codable, Equatable, Sendable {
    public let secret: String
    public let qrCode: String
}

public struct AuthActionResponse: Codable, Equatable, Sendable {
    public let ok: Bool
    public let error: String?
}

public enum AuthChallengeKind: Equatable, Sendable {
    case unauthorized
    case setupRequired
}

public struct AuthChallenge: Error, Equatable, Sendable, LocalizedError {
    public let kind: AuthChallengeKind
    public let message: String

    public init(kind: AuthChallengeKind, message: String) {
        self.kind = kind
        self.message = message
    }

    public var errorDescription: String? {
        message
    }
}

public struct BlackwoodAuthClient: Sendable {
    public let baseURL: URL

    private let session: URLSession

    public init(baseURL: URL, session: URLSession = .shared) {
        self.baseURL = baseURL
        self.session = session
    }

    public func status() async throws -> AuthStatusResponse {
        try await rpc(
            path: "/blackwood.v1.AuthService/Status",
            request: [:]
        )
    }

    public func getSetupInfo() async throws -> AuthSetupInfo {
        try await rpc(
            path: "/blackwood.v1.AuthService/GetSetupInfo",
            request: [:]
        )
    }

    public func confirmSetup(secret: String, code: String) async throws -> AuthActionResponse {
        try await rpc(
            path: "/blackwood.v1.AuthService/ConfirmSetup",
            request: ["secret": secret, "code": code]
        )
    }

    public func login(code: String) async throws -> AuthActionResponse {
        try await rpc(
            path: "/blackwood.v1.AuthService/Login",
            request: ["code": code]
        )
    }

    public func logout() async throws {
        let _: EmptyResponse = try await rpc(
            path: "/blackwood.v1.AuthService/Logout",
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
            let payload = parsedErrorPayload(from: data)
            let message = payload.message ?? String(data: data, encoding: .utf8) ?? "Request failed"
            if http.statusCode == 401 {
                let kind: AuthChallengeKind = payload.code == "setup_required" ? .setupRequired : .unauthorized
                throw AuthChallenge(kind: kind, message: message)
            }
            let disposition: SyncFailureDisposition = http.statusCode >= 500 ? .retryable : .terminal
            throw SyncFailure(message: message, disposition: disposition)
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
}

private struct EmptyResponse: Decodable {}
