package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	blackwoodv1 "github.com/csweichel/blackwood/gen/blackwood/v1"
	"github.com/csweichel/blackwood/internal/rag"
	"github.com/csweichel/blackwood/internal/storage"
)

// ChatHandler implements the ChatService Connect handler.
type ChatHandler struct {
	engine *rag.Engine
	store  *storage.Store
}

// NewChatHandler creates a new ChatHandler.
func NewChatHandler(engine *rag.Engine, store *storage.Store) *ChatHandler {
	return &ChatHandler{engine: engine, store: store}
}

// Chat handles a streaming chat request. It creates or continues a conversation,
// queries the RAG engine, and streams response chunks.
func (h *ChatHandler) Chat(ctx context.Context, req *connect.Request[blackwoodv1.ChatRequest], stream *connect.ServerStream[blackwoodv1.ChatResponse]) error {
	if h.engine == nil {
		return connect.NewError(connect.CodeUnavailable, fmt.Errorf("chat is not available: OpenAI API key not configured"))
	}
	if req.Msg.Message == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("message is required"))
	}

	conversationID := req.Msg.ConversationId
	isNew := conversationID == ""

	// Create or fetch the conversation.
	if isNew {
		title := req.Msg.Message
		if len(title) > 50 {
			title = title[:50]
		}
		conv, err := h.store.CreateConversation(ctx, title)
		if err != nil {
			return connect.NewError(connect.CodeInternal, fmt.Errorf("create conversation: %w", err))
		}
		conversationID = conv.ID
	} else {
		// Verify conversation exists.
		if _, err := h.store.GetConversation(ctx, conversationID); err != nil {
			return connect.NewError(connect.CodeNotFound, fmt.Errorf("conversation not found: %w", err))
		}
	}

	// Save the user message.
	if _, err := h.store.AddMessage(ctx, conversationID, "user", req.Msg.Message, "[]"); err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("save user message: %w", err))
	}

	// Build conversation history from stored messages.
	conv, err := h.store.GetConversation(ctx, conversationID)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("get conversation: %w", err))
	}

	var history []rag.Message
	// Include all messages except the last one (which is the current user message).
	for _, m := range conv.Messages {
		if m.ID == "" {
			continue
		}
		history = append(history, rag.Message{
			Role:    m.Role,
			Content: m.Content,
		})
	}
	// Remove the last message (current user query) from history since Query appends it.
	if len(history) > 0 {
		history = history[:len(history)-1]
	}

	// Query the RAG engine.
	chunks, sources, err := h.engine.Query(ctx, req.Msg.Message, history)
	if err != nil {
		slog.Error("RAG query failed", "error", err)
		return connect.NewError(connect.CodeInternal, fmt.Errorf("query failed: %w", err))
	}

	// Convert sources to proto format.
	protoSources := make([]*blackwoodv1.SourceReference, 0, len(sources))
	for _, s := range sources {
		protoSources = append(protoSources, &blackwoodv1.SourceReference{
			EntryId:       s.EntryID,
			DailyNoteDate: s.DailyNoteDate,
			Snippet:       s.Snippet,
			Score:         s.Score,
		})
	}

	// Stream response chunks.
	var fullResponse strings.Builder
	for chunk := range chunks {
		fullResponse.WriteString(chunk)
		if err := stream.Send(&blackwoodv1.ChatResponse{
			ConversationId: conversationID,
			Content:        chunk,
			Done:           false,
		}); err != nil {
			return err
		}
	}

	// Send final message with sources.
	if err := stream.Send(&blackwoodv1.ChatResponse{
		ConversationId: conversationID,
		Content:        "",
		Done:           true,
		Sources:        protoSources,
	}); err != nil {
		return err
	}

	// Save the assistant message with sources.
	sourcesJSON, _ := json.Marshal(sources)
	if _, err := h.store.AddMessage(ctx, conversationID, "assistant", fullResponse.String(), string(sourcesJSON)); err != nil {
		slog.Error("failed to save assistant message", "error", err)
	}

	return nil
}

// ListConversations returns a paginated list of conversations.
func (h *ChatHandler) ListConversations(ctx context.Context, req *connect.Request[blackwoodv1.ListConversationsRequest]) (*connect.Response[blackwoodv1.ListConversationsResponse], error) {
	if h.engine == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("chat is not available: OpenAI API key not configured"))
	}
	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 50
	}
	offset := int(req.Msg.Offset)

	convos, err := h.store.ListConversations(ctx, limit, offset)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list conversations: %w", err))
	}

	protoConvos := make([]*blackwoodv1.Conversation, 0, len(convos))
	for _, c := range convos {
		protoConvos = append(protoConvos, &blackwoodv1.Conversation{
			Id:        c.ID,
			Title:     c.Title,
			CreatedAt: timestamppb.New(c.CreatedAt),
			UpdatedAt: timestamppb.New(c.UpdatedAt),
		})
	}

	return connect.NewResponse(&blackwoodv1.ListConversationsResponse{
		Conversations: protoConvos,
	}), nil
}

// GetConversation returns a conversation with all its messages.
func (h *ChatHandler) GetConversation(ctx context.Context, req *connect.Request[blackwoodv1.GetConversationRequest]) (*connect.Response[blackwoodv1.Conversation], error) {
	if h.engine == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("chat is not available: OpenAI API key not configured"))
	}
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}

	conv, err := h.store.GetConversation(ctx, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("conversation not found: %w", err))
	}

	protoMessages := make([]*blackwoodv1.ChatMessage, 0, len(conv.Messages))
	for _, m := range conv.Messages {
		// Parse sources JSON into proto format.
		var sources []rag.SourceReference
		_ = json.Unmarshal([]byte(m.Sources), &sources)

		protoSources := make([]*blackwoodv1.SourceReference, 0, len(sources))
		for _, s := range sources {
			protoSources = append(protoSources, &blackwoodv1.SourceReference{
				EntryId:       s.EntryID,
				DailyNoteDate: s.DailyNoteDate,
				Snippet:       s.Snippet,
				Score:         s.Score,
			})
		}

		protoMessages = append(protoMessages, &blackwoodv1.ChatMessage{
			Id:        m.ID,
			Role:      m.Role,
			Content:   m.Content,
			Sources:   protoSources,
			CreatedAt: timestamppb.New(m.CreatedAt),
		})
	}

	return connect.NewResponse(&blackwoodv1.Conversation{
		Id:        conv.ID,
		Title:     conv.Title,
		Messages:  protoMessages,
		CreatedAt: timestamppb.New(conv.CreatedAt),
		UpdatedAt: timestamppb.New(conv.UpdatedAt),
	}), nil
}
