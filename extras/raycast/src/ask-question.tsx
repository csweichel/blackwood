import { Action, ActionPanel, Detail, Form, showToast, Toast } from "@raycast/api";
import { useState } from "react";
import { chatStream, SourceReference } from "./api";

export default function AskQuestion() {
  const [answer, setAnswer] = useState<string | null>(null);
  const [sources, setSources] = useState<SourceReference[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [question, setQuestion] = useState("");

  async function handleSubmit(values: { question: string }) {
    if (!values.question.trim()) {
      await showToast({ style: Toast.Style.Failure, title: "Question cannot be empty" });
      return;
    }

    setQuestion(values.question);
    setIsLoading(true);
    setAnswer(null);
    setSources([]);

    try {
      const result = await chatStream(values.question);
      setAnswer(result.fullResponse);
      setSources(result.sources);
    } catch (error) {
      await showToast({
        style: Toast.Style.Failure,
        title: "Failed to get answer",
        message: String(error),
      });
    } finally {
      setIsLoading(false);
    }
  }

  if (answer !== null) {
    let markdown = answer;

    if (sources.length > 0) {
      markdown += "\n\n---\n\n**Sources:**\n";
      for (const s of sources) {
        const label = s.dailyNoteDate || s.entryId;
        const snippet = s.snippet ? ` — ${s.snippet.slice(0, 100)}` : "";
        markdown += `- ${label}${snippet}\n`;
      }
    }

    return (
      <Detail
        markdown={markdown}
        isLoading={isLoading}
        metadata={
          <Detail.Metadata>
            <Detail.Metadata.Label title="Question" text={question} />
            <Detail.Metadata.Label title="Sources" text={String(sources.length)} />
          </Detail.Metadata>
        }
        actions={
          <ActionPanel>
            <Action.CopyToClipboard title="Copy Answer" content={answer} />
            <Action title="Ask Another" onAction={() => setAnswer(null)} />
          </ActionPanel>
        }
      />
    );
  }

  return (
    <Form
      isLoading={isLoading}
      actions={
        <ActionPanel>
          <Action.SubmitForm title="Ask" onSubmit={handleSubmit} />
        </ActionPanel>
      }
    >
      <Form.TextField id="question" title="Question" placeholder="Ask your notes..." autoFocus />
    </Form>
  );
}
