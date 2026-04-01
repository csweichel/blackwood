import { useState } from "react";
import { usePreferences } from "../hooks/usePreferences";
import { ColorTheme } from "../api/types";

const COMMON_TIMEZONES = [
  "America/New_York",
  "America/Chicago",
  "America/Denver",
  "America/Los_Angeles",
  "America/Anchorage",
  "Pacific/Honolulu",
  "America/Toronto",
  "America/Vancouver",
  "America/Sao_Paulo",
  "Europe/London",
  "Europe/Berlin",
  "Europe/Paris",
  "Europe/Amsterdam",
  "Europe/Zurich",
  "Europe/Rome",
  "Europe/Madrid",
  "Europe/Stockholm",
  "Europe/Helsinki",
  "Europe/Moscow",
  "Asia/Dubai",
  "Asia/Kolkata",
  "Asia/Singapore",
  "Asia/Shanghai",
  "Asia/Tokyo",
  "Asia/Seoul",
  "Australia/Sydney",
  "Australia/Melbourne",
  "Pacific/Auckland",
];

// Build a full list: common timezones first, then all IANA zones from the browser.
function getAllTimezones(): string[] {
  try {
    const all = (Intl as unknown as { supportedValuesOf(key: string): string[] }).supportedValuesOf("timeZone");
    const commonSet = new Set(COMMON_TIMEZONES);
    const rest = all.filter((tz: string) => !commonSet.has(tz));
    return [...COMMON_TIMEZONES, ...rest];
  } catch {
    return COMMON_TIMEZONES;
  }
}

const ALL_TIMEZONES = getAllTimezones();

const themeOptions: { value: ColorTheme; label: string }[] = [
  { value: ColorTheme.SYSTEM, label: "System" },
  { value: ColorTheme.LIGHT, label: "Light" },
  { value: ColorTheme.DARK, label: "Dark" },
];

interface SettingsPanelProps {
  open: boolean;
  onClose: () => void;
}

export default function SettingsPanel({ open, onClose }: SettingsPanelProps) {
  const { preferences, update, detectedTimezone } = usePreferences();
  const [saving, setSaving] = useState(false);

  if (!open) return null;

  const currentTz = preferences.timezone || detectedTimezone;

  async function handleTimezoneChange(tz: string) {
    setSaving(true);
    try {
      await update({ timezone: tz });
    } finally {
      setSaving(false);
    }
  }

  async function handleThemeChange(theme: ColorTheme) {
    setSaving(true);
    try {
      await update({ colorTheme: theme });
    } finally {
      setSaving(false);
    }
  }

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/30 z-40"
        onClick={onClose}
      />
      {/* Modal */}
      <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
        <div className="bg-card border border-border rounded-xl shadow-lg w-full max-w-md">
          <div className="flex items-center justify-between px-5 py-4 border-b border-border">
            <h2 className="text-base font-semibold text-foreground">Settings</h2>
            <button
              onClick={onClose}
              className="p-1.5 text-muted-foreground hover:text-foreground rounded-lg transition-colors"
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M18 6 6 18" /><path d="m6 6 12 12" />
              </svg>
            </button>
          </div>

          <div className="px-5 py-5 space-y-6">
            {/* Timezone */}
            <div>
              <label className="block text-sm font-medium text-foreground mb-1.5">
                Timezone
              </label>
              <p className="text-xs text-muted-foreground mb-2">
                Used for day boundaries and timestamps. Your browser reports <span className="font-medium text-foreground">{detectedTimezone}</span>.
              </p>
              <select
                value={currentTz}
                onChange={(e) => handleTimezoneChange(e.target.value)}
                disabled={saving}
                className="w-full rounded-lg border border-border bg-background text-foreground text-sm px-3 py-2 focus:outline-none focus:ring-2 focus:ring-accent/40"
              >
                {ALL_TIMEZONES.map((tz) => (
                  <option key={tz} value={tz}>
                    {tz.replace(/_/g, " ")}
                  </option>
                ))}
              </select>
            </div>

            {/* Color Theme */}
            <div>
              <label className="block text-sm font-medium text-foreground mb-1.5">
                Color theme
              </label>
              <div className="flex gap-2">
                {themeOptions.map((opt) => (
                  <button
                    key={opt.value}
                    onClick={() => handleThemeChange(opt.value)}
                    disabled={saving}
                    className={`flex-1 px-3 py-2 text-sm font-medium rounded-lg border transition-colors ${
                      preferences.colorTheme === opt.value
                        ? "border-accent bg-accent-subtle text-accent"
                        : "border-border text-muted-foreground hover:text-foreground hover:border-border-strong"
                    }`}
                  >
                    {opt.label}
                  </button>
                ))}
              </div>
            </div>
          </div>
        </div>
      </div>
    </>
  );
}
