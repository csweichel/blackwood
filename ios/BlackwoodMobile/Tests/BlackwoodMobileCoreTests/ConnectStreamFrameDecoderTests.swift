import Foundation
import XCTest
@testable import BlackwoodMobileCore

final class ConnectStreamFrameDecoderTests: XCTestCase {
    func testDecodesConsecutiveFragmentedFramesAfterRemovingTheFirstFrame() {
        var decoder = ConnectStreamFrameDecoder()
        let first = encodedFrame(flags: 0, payload: Data(#"{"result":{"date":"2026-07-13"}}"#.utf8))
        let secondPayload = Data(#"{"result":{"date":"2026-07-14"}}"#.utf8)
        let second = encodedFrame(flags: 2, payload: secondPayload)

        var decoded: [ConnectStreamFrame] = []
        for byte in first + second {
            decoded.append(contentsOf: decoder.append(byte))
        }

        XCTAssertEqual(decoded.count, 2)
        XCTAssertEqual(decoded[0].flags, 0)
        XCTAssertEqual(decoded[1], ConnectStreamFrame(flags: 2, payload: secondPayload))
    }

    func testWaitsForTheCompletePayload() {
        var decoder = ConnectStreamFrameDecoder()
        let payload = Data("payload".utf8)
        let frame = encodedFrame(flags: 0, payload: payload)

        let partialResults = frame.dropLast().flatMap { decoder.append($0) }
        XCTAssertTrue(partialResults.isEmpty)
        XCTAssertEqual(decoder.append(frame.last!), [ConnectStreamFrame(flags: 0, payload: payload)])
    }

    private func encodedFrame(flags: UInt8, payload: Data) -> Data {
        let length = UInt32(payload.count)
        return Data([
            flags,
            UInt8((length >> 24) & 0xff),
            UInt8((length >> 16) & 0xff),
            UInt8((length >> 8) & 0xff),
            UInt8(length & 0xff),
        ]) + payload
    }
}
