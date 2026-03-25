import { showHUD, showToast, Toast } from "@raycast/api";
import { execSync } from "child_process";
import { ingestImage } from "./api";

export default async function IngestImage() {
  // Use macOS file picker via osascript to select an image file.
  let filePath: string;
  try {
    const result = execSync(
      `osascript -e 'POSIX path of (choose file of type {"public.image"} with prompt "Select an image to ingest")'`,
      { encoding: "utf-8", timeout: 60_000 },
    ).trim();
    filePath = result;
  } catch {
    // User cancelled the file picker.
    return;
  }

  if (!filePath) return;

  try {
    await ingestImage(filePath);
    await showHUD("Image ingested");
  } catch (error) {
    await showToast({
      style: Toast.Style.Failure,
      title: "Failed to ingest image",
      message: String(error),
    });
  }
}
