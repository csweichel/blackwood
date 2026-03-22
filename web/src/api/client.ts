import type {
  DailyNote,
  Entry,
  ListDailyNotesResponse,
  CreateEntryRequest,
  DeleteEntryRequest,
  GetDailyNoteRequest,
  ListDailyNotesRequest,
} from "./types";

const SERVICE = "/blackwood.v1.DailyNotesService";

// Connect-go uses POST with JSON body and Content-Type: application/json.
// Field names use camelCase in the JSON wire format (protobuf JSON mapping).
async function rpc<Req, Res>(method: string, request: Req): Promise<Res> {
  const url = `${SERVICE}/${method}`;
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
