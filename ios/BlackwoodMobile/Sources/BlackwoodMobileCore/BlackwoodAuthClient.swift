import Foundation

public struct AuthStatusResponse: Codable, Equatable, Sendable {
    public let authenticated: Bool
    public let setupRequired: Bool

    public init(authenticated: Bool = false, setupRequired: Bool = false) {
        self.authenticated = authenticated
        self.setupRequired = setupRequired
    }

    enum CodingKeys: String, CodingKey {
        case authenticated
        case setupRequired
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        authenticated = try container.decodeIfPresent(Bool.self, forKey: .authenticated) ?? false
        setupRequired = try container.decodeIfPresent(Bool.self, forKey: .setupRequired) ?? false
    }
}

public struct AuthSetupInfo: Codable, Equatable, Sendable {
    public let secret: String
    public let qrCode: String
}

public struct AuthActionResponse: Codable, Equatable, Sendable {
    public let ok: Bool
    public let error: String?

    public init(ok: Bool = false, error: String? = nil) {
        self.ok = ok
        self.error = error
    }

    enum CodingKeys: String, CodingKey {
        case ok
        case error
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        ok = try container.decodeIfPresent(Bool.self, forKey: .ok) ?? false
        error = try container.decodeIfPresent(String.self, forKey: .error)
    }
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

public struct AuthServiceUnavailable: Error, Equatable, Sendable, LocalizedError {
    public init() {}

    public var errorDescription: String? {
        "TOTP authentication is not enabled on this Blackwood server."
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
        do {
            return try await rpc(
                path: "/blackwood.v1.AuthService/Status",
                request: [:]
            )
        } catch is AuthServiceUnavailable {
            return AuthStatusResponse(authenticated: true, setupRequired: false)
        }
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
        try validateJSONResponse(response: response, data: data)
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        return try decoder.decode(Response.self, from: data)
    }

    private func validateJSONResponse(response: URLResponse, data: Data) throws {
        guard let http = response as? HTTPURLResponse else { return }
        let contentType = http.value(forHTTPHeaderField: "Content-Type")?.lowercased() ?? ""
        if contentType.contains("json") {
            return
        }

        let prefix = String(data: data.prefix(128), encoding: .utf8)?.lowercased() ?? ""
        if contentType.contains("text/html") || prefix.contains("<!doctype html") || prefix.contains("<html") {
            throw AuthServiceUnavailable()
        }

        throw SyncFailure(message: "Blackwood returned an unexpected auth response.", disposition: .retryable)
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
