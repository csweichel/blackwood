import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from "react";
import { getPreferences, updatePreferences } from "../api/client";
import { ColorTheme, type UserPreferences, type UpdatePreferencesRequest } from "../api/types";

interface PreferencesContextValue {
  preferences: UserPreferences;
  loading: boolean;
  update: (req: UpdatePreferencesRequest) => Promise<void>;
  /** The browser's detected IANA timezone. */
  detectedTimezone: string;
}

const defaultPreferences: UserPreferences = {
  timezone: "",
  colorTheme: ColorTheme.SYSTEM,
};

const PreferencesContext = createContext<PreferencesContextValue>({
  preferences: defaultPreferences,
  loading: true,
  update: async () => {},
  detectedTimezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
});

export function PreferencesProvider({ children }: { children: ReactNode }) {
  const [preferences, setPreferences] = useState<UserPreferences>(defaultPreferences);
  const [loading, setLoading] = useState(true);
  const detectedTimezone = Intl.DateTimeFormat().resolvedOptions().timeZone;

  useEffect(() => {
    getPreferences()
      .then((p) => setPreferences(p))
      .catch((err) => console.error("Failed to load preferences:", err))
      .finally(() => setLoading(false));
  }, []);

  const update = useCallback(async (req: UpdatePreferencesRequest) => {
    const updated = await updatePreferences(req);
    setPreferences(updated);
  }, []);

  return (
    <PreferencesContext.Provider value={{ preferences, loading, update, detectedTimezone }}>
      {children}
    </PreferencesContext.Provider>
  );
}

export function usePreferences() {
  return useContext(PreferencesContext);
}
