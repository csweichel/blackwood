package api

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	blackwoodv1 "github.com/csweichel/blackwood/gen/blackwood/v1"
	"github.com/csweichel/blackwood/internal/storage"
)

// Preference keys stored in the user_preferences table.
const (
	prefTimezone   = "timezone"
	prefColorTheme = "color_theme"
)

var colorThemeToProto = map[string]blackwoodv1.ColorTheme{
	"light":  blackwoodv1.ColorTheme_COLOR_THEME_LIGHT,
	"dark":   blackwoodv1.ColorTheme_COLOR_THEME_DARK,
	"system": blackwoodv1.ColorTheme_COLOR_THEME_SYSTEM,
}

var protoToColorTheme = map[blackwoodv1.ColorTheme]string{
	blackwoodv1.ColorTheme_COLOR_THEME_LIGHT:  "light",
	blackwoodv1.ColorTheme_COLOR_THEME_DARK:   "dark",
	blackwoodv1.ColorTheme_COLOR_THEME_SYSTEM: "system",
}

// PreferencesHandler implements the PreferencesService Connect handler.
type PreferencesHandler struct {
	store *storage.Store
}

// NewPreferencesHandler creates a new PreferencesHandler.
func NewPreferencesHandler(store *storage.Store) *PreferencesHandler {
	return &PreferencesHandler{store: store}
}

// GetPreferences returns the current user preferences.
func (h *PreferencesHandler) GetPreferences(ctx context.Context, _ *connect.Request[blackwoodv1.GetPreferencesRequest]) (*connect.Response[blackwoodv1.UserPreferences], error) {
	prefs, err := h.loadPreferences(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(prefs), nil
}

// UpdatePreferences updates the user preferences. Only fields that are set in
// the request are updated.
func (h *PreferencesHandler) UpdatePreferences(ctx context.Context, req *connect.Request[blackwoodv1.UpdatePreferencesRequest]) (*connect.Response[blackwoodv1.UserPreferences], error) {
	msg := req.Msg

	if msg.Timezone != nil {
		tz := *msg.Timezone
		if tz != "" {
			if _, err := loadTimezone(tz); err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid timezone: %s", tz))
			}
		}
		if err := h.store.SetPreference(ctx, prefTimezone, tz); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("save timezone: %w", err))
		}
	}

	if msg.ColorTheme != nil {
		theme := *msg.ColorTheme
		if theme == blackwoodv1.ColorTheme_COLOR_THEME_UNSPECIFIED {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("color_theme must be light, dark, or system"))
		}
		str, ok := protoToColorTheme[theme]
		if !ok {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unknown color_theme value: %d", theme))
		}
		if err := h.store.SetPreference(ctx, prefColorTheme, str); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("save color_theme: %w", err))
		}
	}

	prefs, err := h.loadPreferences(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(prefs), nil
}

func (h *PreferencesHandler) loadPreferences(ctx context.Context) (*blackwoodv1.UserPreferences, error) {
	tz, err := h.store.GetPreference(ctx, prefTimezone, "")
	if err != nil {
		return nil, fmt.Errorf("read timezone: %w", err)
	}
	themeStr, err := h.store.GetPreference(ctx, prefColorTheme, "system")
	if err != nil {
		return nil, fmt.Errorf("read color_theme: %w", err)
	}

	theme, ok := colorThemeToProto[themeStr]
	if !ok {
		theme = blackwoodv1.ColorTheme_COLOR_THEME_SYSTEM
	}

	return &blackwoodv1.UserPreferences{
		Timezone:   tz,
		ColorTheme: theme,
	}, nil
}

// loadTimezone validates and loads an IANA timezone name.
func loadTimezone(name string) (*time.Location, error) {
	return time.LoadLocation(name)
}

// UserTimezone returns the configured timezone location, falling back to UTC.
func UserTimezone(ctx context.Context, store *storage.Store) *time.Location {
	tz, err := store.GetPreference(ctx, prefTimezone, "")
	if err != nil || tz == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC
	}
	return loc
}

// UserTimezoneNow returns the current time in the user's configured timezone.
func UserTimezoneNow(ctx context.Context, store *storage.Store) time.Time {
	return time.Now().In(UserTimezone(ctx, store))
}

// UserTimezoneNowDate returns today's date string (YYYY-MM-DD) in the user's timezone.
func UserTimezoneNowDate(ctx context.Context, store *storage.Store) string {
	return UserTimezoneNow(ctx, store).Format("2006-01-02")
}


