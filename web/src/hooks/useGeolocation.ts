import { useState, useCallback } from "react";

interface GeoPosition {
  latitude: number;
  longitude: number;
}

interface UseGeolocationResult {
  position: GeoPosition | null;
  loading: boolean;
  error: string | null;
  requestLocation: () => void;
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

    const onSuccess = (pos: GeolocationPosition) => {
      setPosition({
        latitude: pos.coords.latitude,
        longitude: pos.coords.longitude,
      });
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
