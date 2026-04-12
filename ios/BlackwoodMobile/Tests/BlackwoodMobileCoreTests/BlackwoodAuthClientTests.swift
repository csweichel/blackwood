import Foundation
import XCTest
@testable import BlackwoodMobileCore

final class BlackwoodAuthClientTests: XCTestCase {
    final class MockURLProtocol: URLProtocol {
        static var handler: ((URLRequest) throws -> (HTTPURLResponse, Data))?

        override class func canInit(with request: URLRequest) -> Bool {
            true
        }

        override class func canonicalRequest(for request: URLRequest) -> URLRequest {
            request
        }

        override func startLoading() {
            guard let handler = Self.handler else {
                client?.urlProtocol(self, didFailWithError: URLError(.badServerResponse))
                return
            }

            do {
                let (response, data) = try handler(request)
                client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
                client?.urlProtocol(self, didLoad: data)
                client?.urlProtocolDidFinishLoading(self)
            } catch {
                client?.urlProtocol(self, didFailWithError: error)
            }
        }

        override func stopLoading() {}
    }

    private func makeSession() -> URLSession {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [MockURLProtocol.self]
        return URLSession(configuration: configuration)
    }

    func testFetchDailyNoteMapsSetupRequired401() async throws {
        MockURLProtocol.handler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 401,
                httpVersion: "HTTP/1.1",
                headerFields: ["Content-Type": "application/json"]
            )!
            let data = #"{"code":"setup_required","message":"TOTP setup required"}"#.data(using: .utf8)!
            return (response, data)
        }

        let client = BlackwoodAPIClient(baseURL: URL(string: "http://example.com")!, session: makeSession())

        do {
            _ = try await client.fetchDailyNote(date: "2026-04-11")
            XCTFail("Expected auth challenge")
        } catch let challenge as AuthChallenge {
            XCTAssertEqual(challenge.kind, .setupRequired)
            XCTAssertEqual(challenge.message, "TOTP setup required")
        }
    }

    func testFetchDailyNoteMapsUnauthorized401() async throws {
        MockURLProtocol.handler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 401,
                httpVersion: "HTTP/1.1",
                headerFields: ["Content-Type": "application/json"]
            )!
            let data = #"{"code":"unauthorized","message":"Authentication required"}"#.data(using: .utf8)!
            return (response, data)
        }

        let client = BlackwoodAPIClient(baseURL: URL(string: "http://example.com")!, session: makeSession())

        do {
            _ = try await client.fetchDailyNote(date: "2026-04-11")
            XCTFail("Expected auth challenge")
        } catch let challenge as AuthChallenge {
            XCTAssertEqual(challenge.kind, .unauthorized)
            XCTAssertEqual(challenge.message, "Authentication required")
        }
    }
}
