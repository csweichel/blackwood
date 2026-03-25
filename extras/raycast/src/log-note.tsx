import { Action, ActionPanel, Form, showToast, Toast, popToRoot } from "@raycast/api";
import { useState } from "react";
import { createEntry, EntryType, EntrySource } from "./api";

export default function LogNote() {
  const [isLoading, setIsLoading] = useState(false);

  async function handleSubmit(values: { note: string }) {
    if (!values.note.trim()) {
      await showToast({ style: Toast.Style.Failure, title: "Note cannot be empty" });
      return;
    }

    setIsLoading(true);
    try {
      await createEntry({
        type: EntryType.TEXT,
        source: EntrySource.API,
        content: values.note,
      });
      await showToast({ style: Toast.Style.Success, title: "Note logged" });
      popToRoot();
    } catch (error) {
      await showToast({
        style: Toast.Style.Failure,
        title: "Failed to log note",
        message: String(error),
      });
    } finally {
      setIsLoading(false);
    }
  }

  return (
    <Form
      isLoading={isLoading}
      actions={
        <ActionPanel>
          <Action.SubmitForm title="Log Note" onSubmit={handleSubmit} />
        </ActionPanel>
      }
    >
      <Form.TextArea id="note" title="Note" placeholder="What's on your mind?" autoFocus />
    </Form>
  );
}
