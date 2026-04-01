export const EntryType = {
  UNSPECIFIED: 0,
  TEXT: 1,
  AUDIO: 2,
  PHOTO: 3,
  VIWOODS: 4,
  WEBCLIP: 5,
} as const;
export type EntryType = (typeof EntryType)[keyof typeof EntryType];

export const EntrySource = {
  UNSPECIFIED: 0,
  WEB: 1,
  TELEGRAM: 2,
  WHATSAPP: 3,
  API: 4,
  IMPORT: 5,
} as const;
export type EntrySource = (typeof EntrySource)[keyof typeof EntrySource];

export interface Attachment {
  id: string;
  entryId: string;
  filename: string;
  contentType: string;
  size: number;
  url: string;
}

export interface Entry {
  id: string;
  dailyNoteId: string;
  type: EntryType;
  content: string;
  rawContent: string;
  source: EntrySource;
  metadata: string;
  attachments: Attachment[];
  createdAt: string;
  updatedAt: string;
}

export interface DailyNote {
  id: string;
  date: string;
  content: string;
  entries: Entry[];
  createdAt: string;
  updatedAt: string;
}

export interface ListDailyNotesResponse {
  dailyNotes: DailyNote[];
}

export interface CreateEntryRequest {
  date: string;
  type: EntryType;
  content: string;
  source: EntrySource;
  metadata?: string;
}

export interface DeleteEntryRequest {
  id: string;
}

export interface GetDailyNoteRequest {
  date: string;
}

export interface ListDailyNotesRequest {
  limit?: number;
  offset?: number;
  startDate?: string;
  endDate?: string;
}

export const entryTypeLabel: Record<EntryType, string> = {
  [EntryType.UNSPECIFIED]: "Unknown",
  [EntryType.TEXT]: "Text",
  [EntryType.AUDIO]: "Audio",
  [EntryType.PHOTO]: "Photo",
  [EntryType.VIWOODS]: "Viwoods",
  [EntryType.WEBCLIP]: "Web Clip",
};

export const entrySourceLabel: Record<EntrySource, string> = {
  [EntrySource.UNSPECIFIED]: "Unknown",
  [EntrySource.WEB]: "Web",
  [EntrySource.TELEGRAM]: "Telegram",
  [EntrySource.WHATSAPP]: "WhatsApp",
  [EntrySource.API]: "API",
  [EntrySource.IMPORT]: "Import",
};

// Import job types

export interface ImportJobStatus {
  id: string;
  status: "pending" | "processing" | "done" | "error";
  filename: string;
  fileType: string;   // "viwoods" or "obsidian"
  source: string;     // "upload" or "watcher"
  progress: number;
  totalSteps: number;
  resultJson: string; // JSON string with dates, errors, etc.
  createdAt: string;
  updatedAt: string;
}

// Chat types

export interface SourceReference {
  entryId: string;
  dailyNoteDate: string;
  snippet: string;
  score: number;
}

export interface ChatMessage {
  id: string;
  role: string;
  content: string;
  sources: SourceReference[];
  createdAt: string;
}

export interface Conversation {
  id: string;
  title: string;
  messages: ChatMessage[];
  createdAt: string;
  updatedAt: string;
}

export interface ListConversationsResponse {
  conversations: Conversation[];
}

// Preferences types

/** Proto enum values as returned by Connect-go JSON serialization. */
export const ColorTheme = {
  UNSPECIFIED: "COLOR_THEME_UNSPECIFIED",
  LIGHT: "COLOR_THEME_LIGHT",
  DARK: "COLOR_THEME_DARK",
  SYSTEM: "COLOR_THEME_SYSTEM",
} as const;
export type ColorTheme = (typeof ColorTheme)[keyof typeof ColorTheme];

export interface UserPreferences {
  timezone: string;
  colorTheme: ColorTheme;
}

export interface UpdatePreferencesRequest {
  timezone?: string;
  colorTheme?: ColorTheme;
}
