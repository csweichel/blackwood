package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	blackwoodv1 "github.com/csweichel/blackwood/gen/blackwood/v1"
)

// GetSubpage reads a subpage markdown file from the day directory.
func (h *DailyNotesHandler) GetSubpage(ctx context.Context, req *connect.Request[blackwoodv1.GetSubpageRequest]) (*connect.Response[blackwoodv1.Subpage], error) {
	date := req.Msg.Date
	name := req.Msg.Name
	if date == "" || name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("date and name are required"))
	}

	path, err := h.store.SubpagePath(date, name)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid subpage name: %w", err))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("subpage %q not found", name))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read subpage: %w", err))
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("stat subpage: %w", err))
	}
	modTime := info.ModTime().UTC()

	return connect.NewResponse(&blackwoodv1.Subpage{
		Name:      name,
		Content:   string(data),
		Date:      date,
		Revision:  revisionString(modTime),
		UpdatedAt: timestamppb.New(modTime),
	}), nil
}

// UpdateSubpageContent creates or updates a subpage markdown file.
func (h *DailyNotesHandler) UpdateSubpageContent(ctx context.Context, req *connect.Request[blackwoodv1.UpdateSubpageContentRequest]) (*connect.Response[blackwoodv1.Subpage], error) {
	date := req.Msg.Date
	name := req.Msg.Name
	if date == "" || name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("date and name are required"))
	}

	path, err := h.store.SubpagePath(date, name)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid subpage name: %w", err))
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create directory: %w", err))
	}
	if req.Msg.BaseRevision != "" {
		info, err := os.Stat(path)
		if err == nil {
			currentRevision := revisionString(info.ModTime().UTC())
			if currentRevision != req.Msg.BaseRevision {
				return nil, connect.NewError(
					connect.CodeFailedPrecondition,
					fmt.Errorf("subpage changed on another client; reload and try again"),
				)
			}
		} else if !os.IsNotExist(err) {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("stat subpage: %w", err))
		}
	}

	if err := os.WriteFile(path, []byte(req.Msg.Content), 0o644); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("write subpage: %w", err))
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("stat written subpage: %w", err))
	}
	modTime := info.ModTime().UTC()
	h.changes.PublishSubpage(date, name, modTime)

	return connect.NewResponse(&blackwoodv1.Subpage{
		Name:      name,
		Content:   req.Msg.Content,
		Date:      date,
		Revision:  revisionString(modTime),
		UpdatedAt: timestamppb.New(modTime),
	}), nil
}

// ListSubpages returns the names of all subpage .md files for a given date.
func (h *DailyNotesHandler) ListSubpages(ctx context.Context, req *connect.Request[blackwoodv1.ListSubpagesRequest]) (*connect.Response[blackwoodv1.ListSubpagesResponse], error) {
	date := req.Msg.Date
	if date == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("date is required"))
	}

	names, err := h.store.ListSubpageNames(date)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list subpages: %w", err))
	}

	return connect.NewResponse(&blackwoodv1.ListSubpagesResponse{
		Names: names,
	}), nil
}
