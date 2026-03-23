import type { ImportJobStatus } from "../api/types";
import type { ImportFileResult } from "./ImportModal";

export function jobToFileResult(job: ImportJobStatus): ImportFileResult {
  let message: string | undefined;
  let date: string | undefined;

  if (job.status === "processing") {
    message =
      job.fileType === "viwoods" && job.totalSteps > 0
        ? `Page ${job.progress} of ${job.totalSteps}`
        : "Processing...";
  } else if (job.status === "done") {
    try {
      const result = JSON.parse(job.resultJson);
      date = result.date;
      message =
        job.fileType === "viwoods"
          ? `${result.pages_processed} pages processed`
          : "Imported";
    } catch {
      message = "Done";
    }
  } else if (job.status === "error") {
    try {
      const result = JSON.parse(job.resultJson);
      message = result.error || "Import failed";
    } catch {
      message = "Import failed";
    }
  }

  return {
    id: job.id,
    filename: job.filename,
    status: job.status === "processing" ? "processing" : (job.status as ImportFileResult["status"]),
    message,
    date,
    source: job.source,
  };
}
