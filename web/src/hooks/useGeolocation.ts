import { useState, useCallback } from "react";

interface GeoPosition {
  latitude: number;
  longitude: number;
  /** Human-readable address from reverse geocoding (may be empty). */
  address: string;
}

interface UseGeolocationResult {
  position: GeoPosition | null;
  loading: boolean;
  error: string | null;
  requestLocation: () => void;
}

/**
 * Reverse-geocode coordinates via OpenStreetMap Nominatim.
 * Returns a short human-readable address, or empty string on failure.
 */
async function reverseGeocode(lat: number, lon: number): Promise<string> {
  try {
    const resp = await fetch(
      `https://nominatim.openstreetmap.org/reverse?lat=${lat}&lon=${lon}&format=json&zoom=18&addressdetails=1`,
      { headers: { "User-Agent": "Blackwood/1.0" } },
    );
    if (!resp.ok) return "";
    const data = await resp.json();

    // Build a short address from the most useful fields.
    const a = data.address;
    if (!a) return data.display_name || "";

    const parts: string[] = [];
    // Street-level: road + house number
    if (a.road) {
      parts.push(a.house_number ? `${a.road} ${a.house_number}` : a.road);
    }
    // City/town/village
    const city = a.city || a.town || a.village || a.municipality;
    if (city) parts.push(city);

    if (parts.length > 0) return parts.join(", ");
    return data.display_name || "";
  } catch {
    return "";
  }
}

export function useGeolocation(): UseGeolocationResult {
  const [position, setPosition] = useState<GeoPosition | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const requestLocation = useCallback(() => {
    if (!navigator.geolocation) {
      setError("Geolocation not supported");
      return;
    }

    setLoading(true);
    setError(null);

    const onSuccess = async (pos: GeolocationPosition) => {
      const { latitude, longitude } = pos.coords;
      const address = await reverseGeocode(latitude, longitude);
      setPosition({ latitude, longitude, address });
      setLoading(false);
    };

    // Try high accuracy first, fall back to low accuracy on failure.
    navigator.geolocation.getCurrentPosition(
      onSuccess,
      () => {
        navigator.geolocation.getCurrentPosition(
          onSuccess,
          (err) => {
            setError(
              err.code === 1
                ? "Location permission denied"
                : "Could not determine location"
            );
            setLoading(false);
          },
          { enableHighAccuracy: false, timeout: 15000 }
        );
      },
      { enableHighAccuracy: true, timeout: 10000 }
    );
  }, []);

  return { position, loading, error, requestLocation };
}
