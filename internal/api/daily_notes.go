package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	blackwoodv1 "github.com/csweichel/blackwood/gen/blackwood/v1"
	"github.com/csweichel/blackwood/internal/storage"
)

var entryTypeToProto = map[string]blackwoodv1.EntryType{
	"text":    blackwoodv1.EntryType_ENTRY_TYPE_TEXT,
	"audio":   blackwoodv1.EntryType_ENTRY_TYPE_AUDIO,
	"photo":   blackwoodv1.EntryType_ENTRY_TYPE_PHOTO,
	"viwoods": blackwoodv1.EntryType_ENTRY_TYPE_VIWOODS,
	"webclip": blackwoodv1.EntryType_ENTRY_TYPE_WEBCLIP,
}

var protoToEntryType = map[blackwoodv1.EntryType]string{
	blackwoodv1.EntryType_ENTRY_TYPE_TEXT:    "text",
	blackwoodv1.EntryType_ENTRY_TYPE_AUDIO:   "audio",
	blackwoodv1.EntryType_ENTRY_TYPE_PHOTO:   "photo",
	blackwoodv1.EntryType_ENTRY_TYPE_VIWOODS: "viwoods",
	blackwoodv1.EntryType_ENTRY_TYPE_WEBCLIP: "webclip",
}

var entrySourceToProto = map[string]blackwoodv1.EntrySource{
	"web":      blackwoodv1.EntrySource_ENTRY_SOURCE_WEB,
	"telegram": blackwoodv1.EntrySource_ENTRY_SOURCE_TELEGRAM,
	"whatsapp": blackwoodv1.EntrySource_ENTRY_SOURCE_WHATSAPP,
	"api":      blackwoodv1.EntrySource_ENTRY_SOURCE_API,
	"import":   blackwoodv1.EntrySource_ENTRY_SOURCE_IMPORT,
}

var protoToEntrySource = map[blackwoodv1.EntrySource]string{
	blackwoodv1.EntrySource_ENTRY_SOURCE_WEB:      "web",
	blackwoodv1.EntrySource_ENTRY_SOURCE_TELEGRAM: "telegram",
	blackwoodv1.EntrySource_ENTRY_SOURCE_WHATSAPP: "whatsapp",
	blackwoodv1.EntrySource_ENTRY_SOURCE_API:      "api",
	blackwoodv1.EntrySource_ENTRY_SOURCE_IMPORT:   "import",
}

// DailyNotesHandler implements the DailyNotesService Connect handler.
type DailyNotesHandler struct {
	store *storage.Store
}

// NewDailyNotesHandler creates a new DailyNotesHandler backed by the given store.
func NewDailyNotesHandler(store *storage.Store) *DailyNotesHandler {
	return &DailyNotesHandler{store: store}
}

// GetDailyNote returns the daily note for the given date, including entries and attachments.
func (h *DailyNotesHandler) GetDailyNote(ctx context.Context, req *connect.Request[blackwoodv1.GetDailyNoteRequest]) (*connect.Response[blackwoodv1.DailyNote], error) {
	date := req.Msg.Date
	if date == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("date is required"))
	}

	note, err := h.store.GetOrCreateDailyNote(ctx, date)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get daily note: %w", err))
	}

	protoNote, err := h.dailyNoteToProto(ctx, note, true)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(protoNote), nil
}

// ListDailyNotes returns a paginated list of daily notes.
func (h *DailyNotesHandler) ListDailyNotes(ctx context.Context, req *connect.Request[blackwoodv1.ListDailyNotesRequest]) (*connect.Response[blackwoodv1.ListDailyNotesResponse], error) {
	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 50
	}
	offset := int(req.Msg.Offset)

	notes, err := h.store.ListDailyNotes(ctx, limit, offset)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list daily notes: %w", err))
	}

	protoNotes := make([]*blackwoodv1.DailyNote, 0, len(notes))
	for i := range notes {
		pn, err := h.dailyNoteToProto(ctx, &notes[i], false)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		protoNotes = append(protoNotes, pn)
	}

	return connect.NewResponse(&blackwoodv1.ListDailyNotesResponse{
		DailyNotes: protoNotes,
	}), nil
}

// CreateEntry creates a new entry in the daily note for the given date.
func (h *DailyNotesHandler) CreateEntry(ctx context.Context, req *connect.Request[blackwoodv1.CreateEntryRequest]) (*connect.Response[blackwoodv1.Entry], error) {
	date := req.Msg.Date
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}

	note, err := h.store.GetOrCreateDailyNote(ctx, date)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get or create daily note: %w", err))
	}

	entry := &storage.Entry{
		DailyNoteID: note.ID,
		Type:        protoToEntryType[req.Msg.Type],
		Content:     req.Msg.Content,
		RawContent:  req.Msg.Content,
		Source:      protoToEntrySource[req.Msg.Source],
		Metadata:    req.Msg.Metadata,
	}
	if err := h.store.CreateEntry(ctx, entry); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create entry: %w", err))
	}

	// Handle attachments.
	for i, data := range req.Msg.AttachmentData {
		att := &storage.Attachment{
			EntryID: entry.ID,
		}
		if i < len(req.Msg.AttachmentFilenames) {
			att.Filename = req.Msg.AttachmentFilenames[i]
		}
		if i < len(req.Msg.AttachmentContentTypes) {
			att.ContentType = req.Msg.AttachmentContentTypes[i]
		}
		if err := h.store.CreateAttachment(ctx, att, data); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create attachment: %w", err))
		}
	}

	// Append to the daily note's markdown content.
	now := time.Now().UTC()
	ts := now.Format("15:04")
	var snippet string
	switch entry.Type {
	case "audio":
		snippet = fmt.Sprintf("\n\n---\n*%s — Audio recording*\n\n%s\n", ts, entry.Content)
	case "photo":
		snippet = fmt.Sprintf("\n\n---\n*%s — Photo*\n\n%s\n", ts, entry.Content)
	case "viwoods":
		snippet = fmt.Sprintf("\n\n---\n*%s — Viwoods note*\n\n%s\n", ts, entry.Content)
	default:
		snippet = fmt.Sprintf("\n\n---\n*%s*\n\n%s\n", ts, entry.Content)
	}
	if err := h.store.AppendDailyNoteContent(ctx, note.ID, snippet); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("append daily note content: %w", err))
	}

	protoEntry, err := h.entryToProto(ctx, entry)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(protoEntry), nil
}

// UpdateEntry updates an existing entry's content and metadata.
func (h *DailyNotesHandler) UpdateEntry(ctx context.Context, req *connect.Request[blackwoodv1.UpdateEntryRequest]) (*connect.Response[blackwoodv1.Entry], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}

	entry, err := h.store.GetEntry(ctx, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("get entry: %w", err))
	}

	entry.Content = req.Msg.Content
	entry.Metadata = req.Msg.Metadata

	if err := h.store.UpdateEntry(ctx, entry); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update entry: %w", err))
	}

	protoEntry, err := h.entryToProto(ctx, entry)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(protoEntry), nil
}

// DeleteEntry removes an entry by ID.
func (h *DailyNotesHandler) DeleteEntry(ctx context.Context, req *connect.Request[blackwoodv1.DeleteEntryRequest]) (*connect.Response[blackwoodv1.DeleteEntryResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}

	if err := h.store.DeleteEntry(ctx, req.Msg.Id); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("delete entry: %w", err))
	}

	return connect.NewResponse(&blackwoodv1.DeleteEntryResponse{}), nil
}

// ListEntries returns all entries for a given daily note.
func (h *DailyNotesHandler) ListEntries(ctx context.Context, req *connect.Request[blackwoodv1.ListEntriesRequest]) (*connect.Response[blackwoodv1.ListEntriesResponse], error) {
	if req.Msg.DailyNoteId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("daily_note_id is required"))
	}

	entries, err := h.store.ListEntries(ctx, req.Msg.DailyNoteId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list entries: %w", err))
	}

	protoEntries := make([]*blackwoodv1.Entry, 0, len(entries))
	for i := range entries {
		pe, err := h.entryToProto(ctx, &entries[i])
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		protoEntries = append(protoEntries, pe)
	}

	return connect.NewResponse(&blackwoodv1.ListEntriesResponse{
		Entries: protoEntries,
	}), nil
}

// UpdateDailyNoteContent replaces the markdown content of a daily note.
func (h *DailyNotesHandler) UpdateDailyNoteContent(ctx context.Context, req *connect.Request[blackwoodv1.UpdateDailyNoteContentRequest]) (*connect.Response[blackwoodv1.DailyNote], error) {
	date := req.Msg.Date
	if date == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("date is required"))
	}

	note, err := h.store.GetOrCreateDailyNote(ctx, date)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get or create daily note: %w", err))
	}

	if err := h.store.UpdateDailyNoteContent(ctx, note.ID, req.Msg.Content); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update daily note content: %w", err))
	}

	// Re-fetch to get updated timestamps.
	note, err = h.store.GetDailyNote(ctx, note.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get daily note: %w", err))
	}

	protoNote, err := h.dailyNoteToProto(ctx, note, true)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(protoNote), nil
}

// ListDatesWithContent returns dates that have non-empty markdown content within a date range.
func (h *DailyNotesHandler) ListDatesWithContent(ctx context.Context, req *connect.Request[blackwoodv1.ListDatesWithContentRequest]) (*connect.Response[blackwoodv1.ListDatesWithContentResponse], error) {
	dates, err := h.store.ListDatesWithContent(ctx, req.Msg.StartDate, req.Msg.EndDate)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list dates with content: %w", err))
	}
	return connect.NewResponse(&blackwoodv1.ListDatesWithContentResponse{
		Dates: dates,
	}), nil
}

// ServeAttachment serves attachment file data over HTTP.
func ServeAttachment(store *storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "missing attachment id", http.StatusBadRequest)
			return
		}

		att, err := store.GetAttachment(r.Context(), id)
		if err != nil {
			http.Error(w, "attachment not found", http.StatusNotFound)
			return
		}

		data, err := store.GetAttachmentData(r.Context(), id)
		if err != nil {
			http.Error(w, "failed to read attachment", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", att.ContentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", att.Filename))
		w.Write(data)
	}
}

// --- conversion helpers ---

func (h *DailyNotesHandler) dailyNoteToProto(ctx context.Context, n *storage.DailyNote, includeEntries bool) (*blackwoodv1.DailyNote, error) {
	pn := &blackwoodv1.DailyNote{
		Id:        n.ID,
		Date:      n.Date,
		Content:   n.Content,
		CreatedAt: timestamppb.New(n.CreatedAt),
		UpdatedAt: timestamppb.New(n.UpdatedAt),
	}

	if includeEntries {
		entries, err := h.store.ListEntries(ctx, n.ID)
		if err != nil {
			return nil, fmt.Errorf("list entries for note %s: %w", n.ID, err)
		}
		for i := range entries {
			pe, err := h.entryToProto(ctx, &entries[i])
			if err != nil {
				return nil, err
			}
			pn.Entries = append(pn.Entries, pe)
		}
	}

	return pn, nil
}

func (h *DailyNotesHandler) entryToProto(ctx context.Context, e *storage.Entry) (*blackwoodv1.Entry, error) {
	pe := &blackwoodv1.Entry{
		Id:          e.ID,
		DailyNoteId: e.DailyNoteID,
		Type:        entryTypeToProto[e.Type],
		Content:     e.Content,
		RawContent:  e.RawContent,
		Source:      entrySourceToProto[e.Source],
		Metadata:    e.Metadata,
		CreatedAt:   timestamppb.New(e.CreatedAt),
		UpdatedAt:   timestamppb.New(e.UpdatedAt),
	}

	attachments, err := h.store.ListAttachments(ctx, e.ID)
	if err != nil {
		return nil, fmt.Errorf("list attachments for entry %s: %w", e.ID, err)
	}
	for _, a := range attachments {
		pe.Attachments = append(pe.Attachments, &blackwoodv1.Attachment{
			Id:          a.ID,
			EntryId:     a.EntryID,
			Filename:    a.Filename,
			ContentType: a.ContentType,
			Size:        a.Size,
			Url:         fmt.Sprintf("/api/attachments/%s", a.ID),
		})
	}

	return pe, nil
}
