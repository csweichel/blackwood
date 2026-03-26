import Foundation

public struct APIAttachment: Codable, Equatable, Sendable, Identifiable {
    public let id: String
    public let entryId: String
    public let filename: String
    public let contentType: String
    public let size: Int
    public let url: String

    enum CodingKeys: String, CodingKey {
        case id
        case entryId
        case filename
        case contentType
        case size
        case url
    }

    public init(
        id: String,
        entryId: String,
        filename: String,
        contentType: String,
        size: Int,
        url: String
    ) {
        self.id = id
        self.entryId = entryId
        self.filename = filename
        self.contentType = contentType
        self.size = size
        self.url = url
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        entryId = try container.decode(String.self, forKey: .entryId)
        filename = try container.decodeIfPresent(String.self, forKey: .filename) ?? ""
        contentType = try container.decodeIfPresent(String.self, forKey: .contentType) ?? ""
        if let intValue = try? container.decode(Int.self, forKey: .size) {
            size = intValue
        } else if let stringValue = try? container.decode(String.self, forKey: .size) {
            size = Int(stringValue) ?? 0
        } else {
            size = 0
        }
        url = try container.decodeIfPresent(String.self, forKey: .url) ?? ""
    }
}

public struct APIEntry: Codable, Equatable, Sendable, Identifiable {
    public let id: String
    public let dailyNoteId: String
    public let type: Int
    public let content: String
    public let rawContent: String
    public let source: Int
    public let metadata: String
    public let attachments: [APIAttachment]
    public let createdAt: String
    public let updatedAt: String

    enum CodingKeys: String, CodingKey {
        case id
        case dailyNoteId
        case type
        case content
        case rawContent
        case source
        case metadata
        case attachments
        case createdAt
        case updatedAt
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        dailyNoteId = try container.decode(String.self, forKey: .dailyNoteId)
        type = try APIEntry.decodeEnumValue(from: container, forKey: .type, mapping: [
            "ENTRY_TYPE_UNSPECIFIED": 0,
            "ENTRY_TYPE_TEXT": 1,
            "ENTRY_TYPE_AUDIO": 2,
            "ENTRY_TYPE_PHOTO": 3,
            "ENTRY_TYPE_VIWOODS": 4,
            "ENTRY_TYPE_WEBCLIP": 5,
        ])
        content = try container.decodeIfPresent(String.self, forKey: .content) ?? ""
        rawContent = try container.decodeIfPresent(String.self, forKey: .rawContent) ?? content
        source = try APIEntry.decodeEnumValue(from: container, forKey: .source, mapping: [
            "ENTRY_SOURCE_UNSPECIFIED": 0,
            "ENTRY_SOURCE_WEB": 1,
            "ENTRY_SOURCE_TELEGRAM": 2,
            "ENTRY_SOURCE_WHATSAPP": 3,
            "ENTRY_SOURCE_API": 4,
            "ENTRY_SOURCE_IMPORT": 5,
        ])
        metadata = try container.decodeIfPresent(String.self, forKey: .metadata) ?? ""
        attachments = try container.decodeIfPresent([APIAttachment].self, forKey: .attachments) ?? []
        createdAt = try container.decodeIfPresent(String.self, forKey: .createdAt) ?? ""
        updatedAt = try container.decodeIfPresent(String.self, forKey: .updatedAt) ?? ""
    }

    public init(
        id: String,
        dailyNoteId: String,
        type: Int,
        content: String,
        rawContent: String,
        source: Int,
        metadata: String,
        attachments: [APIAttachment],
        createdAt: String,
        updatedAt: String
    ) {
        self.id = id
        self.dailyNoteId = dailyNoteId
        self.type = type
        self.content = content
        self.rawContent = rawContent
        self.source = source
        self.metadata = metadata
        self.attachments = attachments
        self.createdAt = createdAt
        self.updatedAt = updatedAt
    }

    private static func decodeEnumValue(
        from container: KeyedDecodingContainer<CodingKeys>,
        forKey key: CodingKeys,
        mapping: [String: Int]
    ) throws -> Int {
        if let intValue = try? container.decode(Int.self, forKey: key) {
            return intValue
        }
        let stringValue = try container.decode(String.self, forKey: key)
        return mapping[stringValue] ?? 0
    }
}

public struct APIDailyNote: Codable, Equatable, Sendable {
    public let id: String
    public let date: String
    public let entries: [APIEntry]
    public let createdAt: String
    public let updatedAt: String
    public let content: String

    enum CodingKeys: String, CodingKey {
        case id
        case date
        case entries
        case createdAt
        case updatedAt
        case content
    }

    public init(
        id: String,
        date: String,
        entries: [APIEntry],
        createdAt: String,
        updatedAt: String,
        content: String
    ) {
        self.id = id
        self.date = date
        self.entries = entries
        self.createdAt = createdAt
        self.updatedAt = updatedAt
        self.content = content
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        date = try container.decode(String.self, forKey: .date)
        entries = try container.decodeIfPresent([APIEntry].self, forKey: .entries) ?? []
        createdAt = try container.decodeIfPresent(String.self, forKey: .createdAt) ?? ""
        updatedAt = try container.decodeIfPresent(String.self, forKey: .updatedAt) ?? ""
        content = try container.decodeIfPresent(String.self, forKey: .content) ?? ""
    }
}

public struct SearchResult: Codable, Equatable, Sendable, Identifiable {
    public let entryId: String
    public let date: String
    public let snippet: String
    public let score: Double

    public var id: String { entryId + "|" + date }
}

public struct SearchResponse: Codable, Equatable, Sendable {
    public let results: [SearchResult]
}

public struct CachedDailyNote: Codable, Equatable, Sendable {
    public let date: String
    public var content: String
    public var updatedAt: Date
}

public struct PendingNoteUpdate: Codable, Equatable, Sendable, Identifiable {
    public let id: String
    public let date: String
    public var content: String
    public var updatedAt: Date

    public init(id: String = UUID().uuidString, date: String, content: String, updatedAt: Date = Date()) {
        self.id = id
        self.date = date
        self.content = content
        self.updatedAt = updatedAt
    }
}

public enum PendingUploadStatus: String, Codable, Equatable, Sendable {
    case pending
    case uploading
    case failed
}

public struct PendingEntryUpload: Codable, Equatable, Sendable, Identifiable {
    public let id: String
    public let clientRequestId: String
    public let date: String
    public let content: String
    public let source: Int
    public let type: Int
    public let localFilePath: String
    public let filename: String
    public let contentType: String
    public let createdAt: Date
    public let capturedAt: Date
    public let duration: TimeInterval
    public var status: PendingUploadStatus
    public var attemptCount: Int
    public var lastError: String?
    public var nextRetryAt: Date?

    public init(
        id: String = UUID().uuidString,
        clientRequestId: String = UUID().uuidString,
        date: String,
        content: String = "",
        source: Int = 4,
        type: Int = 2,
        localFilePath: String,
        filename: String = "recording.m4a",
        contentType: String = "audio/x-m4a",
        createdAt: Date = Date(),
        capturedAt: Date = Date(),
        duration: TimeInterval,
        status: PendingUploadStatus = .pending,
        attemptCount: Int = 0,
        lastError: String? = nil,
        nextRetryAt: Date? = nil
    ) {
        self.id = id
        self.clientRequestId = clientRequestId
        self.date = date
        self.content = content
        self.source = source
        self.type = type
        self.localFilePath = localFilePath
        self.filename = filename
        self.contentType = contentType
        self.createdAt = createdAt
        self.capturedAt = capturedAt
        self.duration = duration
        self.status = status
        self.attemptCount = attemptCount
        self.lastError = lastError
        self.nextRetryAt = nextRetryAt
    }
}

public struct QueueSnapshot: Equatable, Sendable {
    public let noteUpdateCount: Int
    public let uploadCount: Int
    public let failedUploadCount: Int
}

public enum SyncFailureDisposition: Equatable, Sendable {
    case retryable
    case terminal
}

public struct SyncFailure: Error, Equatable, Sendable {
    public let message: String
    public let disposition: SyncFailureDisposition

    public init(message: String, disposition: SyncFailureDisposition) {
        self.message = message
        self.disposition = disposition
    }
}
