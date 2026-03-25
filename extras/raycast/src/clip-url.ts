import { Clipboard, showHUD, showToast, Toast } from "@raycast/api";
import { clipUrl } from "./api";

export default async function ClipURL() {
  const text = await Clipboard.readText();
  if (!text) {
    await showToast({ style: Toast.Style.Failure, title: "Clipboard is empty" });
    return;
  }

  const trimmed = text.trim();
  try {
    new URL(trimmed);
  } catch {
    await showToast({
      style: Toast.Style.Failure,
      title: "Clipboard does not contain a valid URL",
      message: trimmed.slice(0, 80),
    });
    return;
  }

  try {
    const result = await clipUrl(trimmed);
    await showHUD(`Clipped to ${result.date}`);
  } catch (error) {
    await showToast({
      style: Toast.Style.Failure,
      title: "Failed to clip URL",
      message: String(error),
    });
  }
}
