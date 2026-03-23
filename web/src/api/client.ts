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
} from "./types";
import { type EntryType, type EntrySource } from "./types";

const DAILY_NOTES_SERVICE = "/blackwood.v1.DailyNotesService";
const CHAT_SERVICE = "/blackwood.v1.ChatService";
const IMPORT_SERVICE = "/blackwood.v1.ImportService";

// Connect-go uses POST with JSON body and Content-Type: application/json.
// Field names use camelCase in the JSON wire format (protobuf JSON mapping).
async function rpc<Req, Res>(method: string, request: Req, service: string = DAILY_NOTES_SERVICE): Promise<Res> {
  const url = `${service}/${method}`;
  const resp = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(request),
  });
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`RPC ${method} failed (${resp.status}): ${text}`);
  }
  return resp.json();
}

export async function getDailyNote(req: GetDailyNoteRequest): Promise<DailyNote> {
  return rpc<GetDailyNoteRequest, DailyNote>("GetDailyNote", req);
}

export async function listDailyNotes(req: ListDailyNotesRequest): Promise<ListDailyNotesResponse> {
  return rpc<ListDailyNotesRequest, ListDailyNotesResponse>("ListDailyNotes", req);
}

export async function createEntry(req: CreateEntryRequest): Promise<Entry> {
  return rpc<CreateEntryRequest, Entry>("CreateEntry", req);
}

export async function deleteEntry(req: DeleteEntryRequest): Promise<void> {
  await rpc<DeleteEntryRequest, Record<string, never>>("DeleteEntry", req);
}

export async function updateDailyNoteContent(date: string, content: string): Promise<DailyNote> {
  return rpc<{ date: string; content: string }, DailyNote>("UpdateDailyNoteContent", { date, content });
}

export async function listDatesWithContent(startDate: string, endDate: string): Promise<{ dates: string[] }> {
  return rpc<{ startDate: string; endDate: string }, { dates: string[] }>("ListDatesWithContent", { startDate, endDate });
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

  return rpc<Record<string, unknown>, Entry>("CreateEntry", {
    date,
    type,
    content,
    source,
    attachmentData: [base64],
    attachmentFilenames: [file.name],
    attachmentContentTypes: [file.type],
  });
}

// Import API

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

  const reader = resp.body!.getReader();
  const decoder = new TextDecoder();
  let pending = new Uint8Array(0);

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    const next = new Uint8Array(pending.length + value.length);
    next.set(pending);
    next.set(value, pending.length);
    pending = next;

    // Parse complete envelopes from the buffer.
    while (pending.length >= 5) {
      const flags = pending[0];
      const len = new DataView(pending.buffer, pending.byteOffset).getUint32(1, false);
      if (pending.length < 5 + len) break;

      const payload = pending.slice(5, 5 + len);
      pending = pending.slice(5 + len);

      // Flags 0x02 = end-of-stream trailers; skip.
      if (flags & 0x02) continue;

      const parsed = JSON.parse(decoder.decode(payload));
      yield parsed;
    }
  }
}
