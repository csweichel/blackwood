import type {
  DailyNote,
  Entry,
  ListDailyNotesResponse,
  CreateEntryRequest,
  DeleteEntryRequest,
  GetDailyNoteRequest,
  ListDailyNotesRequest,
  ListConversationsResponse,
  Conversation,
  SourceReference,
  ImportJobStatus,
  UserPreferences,
  UpdatePreferencesRequest,
} from "./types";
import { type EntryType, type EntrySource } from "./types";
import {
  cacheDailyNote,
  getCachedDailyNote,
  queueEntry,
  queueContentUpdate,
} from "../lib/offlineStore";
import { notifyPendingChange } from "../lib/syncEngine";

const DAILY_NOTES_SERVICE = "/blackwood.v1.DailyNotesService";
const CHAT_SERVICE = "/blackwood.v1.ChatService";
const IMPORT_SERVICE = "/blackwood.v1.ImportService";
const PREFERENCES_SERVICE = "/blackwood.v1.PreferencesService";

// Connect-go uses POST with JSON body and Content-Type: application/json.
// Field names use camelCase in the JSON wire format (protobuf JSON mapping).
async function rpc<Req, Res>(method: string, request: Req, service: string = DAILY_NOTES_SERVICE): Promise<Res> {
  const url = `${service}/${method}`;
  const resp = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(request),
  });
  if (resp.status === 401) {
    if (resp.headers.get("X-Auth-Setup-Required") === "true") {
      window.location.href = "/auth/setup";
    } else {
      window.location.href = "/auth/login";
    }
    throw new Error("Unauthorized");
  }
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`RPC ${method} failed (${resp.status}): ${text}`);
  }
  return resp.json();
}

// --- Offline-aware API functions ---

export async function getDailyNote(req: GetDailyNoteRequest): Promise<DailyNote> {
  try {
    const data = await rpc<GetDailyNoteRequest, DailyNote>("GetDailyNote", req);
    // Cache on success.
    await cacheDailyNote(req.date, data.content ?? "");
    return data;
  } catch (err) {
    if (!navigator.onLine) {
      const cached = await getCachedDailyNote(req.date);
      if (cached) {
        return {
          id: "",
          date: cached.date,
          content: cached.content,
          entries: [],
          createdAt: "",
          updatedAt: new Date(cached.updatedAt).toISOString(),
        };
      }
    }
    throw err;
  }
}

export async function listDailyNotes(req: ListDailyNotesRequest): Promise<ListDailyNotesResponse> {
  try {
    return await rpc<ListDailyNotesRequest, ListDailyNotesResponse>("ListDailyNotes", req);
  } catch {
    if (!navigator.onLine) {
      return { dailyNotes: [] };
    }
    throw new Error("Failed to list daily notes");
  }
}

export async function createEntry(req: CreateEntryRequest): Promise<Entry> {
  if (navigator.onLine) {
    try {
      return await rpc<CreateEntryRequest, Entry>("CreateEntry", req);
    } catch {
      // Network error despite onLine — fall through to queue.
    }
  }
  // Offline: queue for later sync.
  const id = await queueEntry({
    date: req.date,
    type: req.type,
    content: req.content,
    source: req.source,
    createdAt: Date.now(),
  });
  await notifyPendingChange();
  return {
    id: `pending-${id}`,
    dailyNoteId: "",
    type: req.type,
    content: req.content,
    rawContent: req.content,
    source: req.source,
    metadata: "",
    attachments: [],
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
  };
}

export async function deleteEntry(req: DeleteEntryRequest): Promise<void> {
  await rpc<DeleteEntryRequest, Record<string, never>>("DeleteEntry", req);
}

export async function updateDailyNoteContent(date: string, content: string): Promise<DailyNote> {
  // Always update the local cache optimistically.
  await cacheDailyNote(date, content);

  if (navigator.onLine) {
    try {
      return await rpc<{ date: string; content: string }, DailyNote>("UpdateDailyNoteContent", { date, content });
    } catch {
      // Network error despite onLine — queue it.
      await queueContentUpdate(date, content);
      await notifyPendingChange();
      return { id: "", date, content, entries: [], createdAt: "", updatedAt: new Date().toISOString() };
    }
  }

  // Offline: queue for later sync.
  await queueContentUpdate(date, content);
  await notifyPendingChange();
  return { id: "", date, content, entries: [], createdAt: "", updatedAt: new Date().toISOString() };
}

export async function listDatesWithContent(startDate: string, endDate: string): Promise<{ dates: string[] }> {
  try {
    return await rpc<{ startDate: string; endDate: string }, { dates: string[] }>("ListDatesWithContent", { startDate, endDate });
  } catch {
    if (!navigator.onLine) {
      return { dates: [] };
    }
    throw new Error("Failed to list dates with content");
  }
}

export async function createEntryWithAttachment(
  date: string,
  type: EntryType,
  content: string,
  source: EntrySource,
  file: File
): Promise<Entry> {
  const buffer = await file.arrayBuffer();
  const bytes = new Uint8Array(buffer);
  let binary = "";
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  const base64 = btoa(binary);

  if (navigator.onLine) {
    try {
      return await rpc<Record<string, unknown>, Entry>("CreateEntry", {
        date, type, content, source,
        attachmentData: [base64],
        attachmentFilenames: [file.name],
        attachmentContentTypes: [file.type],
      });
    } catch {
      // Fall through to queue.
    }
  }

  // Offline: queue with attachment data.
  const id = await queueEntry({
    date, type, content, source,
    attachmentData: [base64],
    attachmentFilenames: [file.name],
    attachmentContentTypes: [file.type],
    createdAt: Date.now(),
  });
  await notifyPendingChange();
  return {
    id: `pending-${id}`,
    dailyNoteId: "",
    type, content, rawContent: content, source,
    metadata: "",
    attachments: [],
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
  };
}

// --- Range summaries (plain HTTP, not Connect-go RPC) ---

export async function fetchSummariesInRange(start: string, end: string): Promise<{ date: string; summary: string }[]> {
  const resp = await fetch(`/api/daily-notes/range?start=${start}&end=${end}`);
  if (!resp.ok) throw new Error("Failed to fetch summaries");
  return resp.json();
}

// --- Import API (no offline support — imports require the server) ---

export async function importObsidian(files: File[]): Promise<{imported: number, skipped: number, errors: string[]}> {
  const obsidianFiles = await Promise.all(files.map(async (f) => {
    const buffer = await f.arrayBuffer();
    const bytes = new Uint8Array(buffer);
    let binary = "";
    for (let i = 0; i < bytes.length; i++) {
      binary += String.fromCharCode(bytes[i]);
    }
    return { filename: f.name, content: btoa(binary) };
  }));
  return rpc<{files: typeof obsidianFiles}, {imported: number, skipped: number, errors: string[]}>(
    "ImportObsidian", { files: obsidianFiles }, IMPORT_SERVICE
  );
}

export async function importViwoods(file: File): Promise<{dailyNoteId: string, entryId: string, pagesProcessed: number}> {
  const buffer = await file.arrayBuffer();
  const bytes = new Uint8Array(buffer);
  let binary = "";
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  return rpc<{noteFile: string, filename: string}, {dailyNoteId: string, entryId: string, pagesProcessed: number}>(
    "ImportViwoods", { noteFile: btoa(binary), filename: file.name }, IMPORT_SERVICE
  );
}

// Background import API

export async function submitImport(files: File[]): Promise<{jobIds: string[]}> {
  const importFiles = await Promise.all(files.map(async (f) => {
    const buffer = await f.arrayBuffer();
    const bytes = new Uint8Array(buffer);
    let binary = "";
    for (let i = 0; i < bytes.length; i++) {
      binary += String.fromCharCode(bytes[i]);
    }
    return { filename: f.name, content: btoa(binary) };
  }));
  return rpc<{files: typeof importFiles}, {jobIds: string[]}>(
    "SubmitImport", { files: importFiles }, IMPORT_SERVICE
  );
}

export async function getImportJobs(
  ids?: string[],
  activeOnly?: boolean
): Promise<{jobs: ImportJobStatus[]}> {
  return rpc<{ids?: string[], activeOnly?: boolean}, {jobs: ImportJobStatus[]}>(
    "GetImportJobs", { ids, activeOnly }, IMPORT_SERVICE
  );
}

export async function deleteImportJob(id: string): Promise<void> {
  await rpc<{id: string}, Record<string, never>>(
    "DeleteImportJob", { id }, IMPORT_SERVICE
  );
}

// Chat API

export async function listConversations(limit = 50, offset = 0): Promise<ListConversationsResponse> {
  return rpc<{ limit: number; offset: number }, ListConversationsResponse>(
    "ListConversations",
    { limit, offset },
    CHAT_SERVICE
  );
}

export async function getConversation(id: string): Promise<Conversation> {
  return rpc<{ id: string }, Conversation>("GetConversation", { id }, CHAT_SERVICE);
}

// Connect protocol envelope: 1 byte flags + 4 byte big-endian length + payload.
function envelopeRequest(payload: object): Blob {
  const json = new TextEncoder().encode(JSON.stringify(payload));
  const header = new Uint8Array(5);
  header[0] = 0; // flags: uncompressed
  new DataView(header.buffer).setUint32(1, json.length, false);
  return new Blob([header, json]);
}

// Streaming chat using Connect server-streaming protocol.
export async function* streamChat(
  conversationId: string,
  message: string
): AsyncGenerator<{ content: string; done: boolean; conversationId: string; sources: SourceReference[] }> {
  const resp = await fetch(`${CHAT_SERVICE}/Chat`, {
    method: "POST",
    headers: {
      "Content-Type": "application/connect+json",
      "Connect-Protocol-Version": "1",
    },
    body: envelopeRequest({ conversationId, message }),
  });
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`Chat failed (${resp.status}): ${text}`);
  }
  if (!resp.body) {
    throw new Error("Chat failed: empty response body");
  }

  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let buffer = new Uint8Array(0);

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    if (!value || value.length === 0) continue;

    const merged = new Uint8Array(buffer.length + value.length);
    merged.set(buffer, 0);
    merged.set(value, buffer.length);
    buffer = merged;

    while (buffer.length >= 5) {
      const flags = buffer[0];
      const len =
        (((buffer[1] << 24) |
        (buffer[2] << 16) |
        (buffer[3] << 8) |
        buffer[4]) >>> 0);
      const frameSize = 5 + len;
      if (buffer.length < frameSize) break;

      const payload = buffer.slice(5, frameSize);
      buffer = buffer.slice(frameSize);

      if ((flags & 0x01) !== 0) {
        throw new Error("Chat failed: compressed stream frames are not supported");
      }

      const text = decoder.decode(payload);
      if (!text.trim()) continue;
      const parsed = JSON.parse(text) as Record<string, unknown>;

      // End-stream envelope can carry protocol-level errors.
      if ((flags & 0x02) !== 0) {
        const streamErr = parsed.error as { message?: string; code?: string } | undefined;
        if (streamErr?.message) {
          throw new Error(`Chat failed (${streamErr.code ?? "unknown"}): ${streamErr.message}`);
        }
        continue;
      }

      const msg = (parsed.result ?? parsed) as {
        content?: string;
        done?: boolean;
        conversationId?: string;
        sources?: SourceReference[];
      };

      yield {
        content: msg.content ?? "",
        done: Boolean(msg.done),
        conversationId: msg.conversationId ?? "",
        sources: msg.sources ?? [],
      };
    }
  }
}

// --- Preferences API ---

export async function getPreferences(): Promise<UserPreferences> {
  return rpc<Record<string, never>, UserPreferences>("GetPreferences", {}, PREFERENCES_SERVICE);
}

export async function updatePreferences(req: UpdatePreferencesRequest): Promise<UserPreferences> {
  return rpc<UpdatePreferencesRequest, UserPreferences>("UpdatePreferences", req, PREFERENCES_SERVICE);
}

