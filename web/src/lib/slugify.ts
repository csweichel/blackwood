export function conversationSlug(conv: { id: string; title: string; createdAt: string }): string {
  const datePrefix = conv.createdAt?.slice(0, 10) || "";
  if (conv.title) {
    const slug = conv.title
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-|-$/g, "")
      .slice(0, 60);
    return `${datePrefix}-${slug}`;
  }
  return `${datePrefix}-${conv.id.slice(0, 8)}`;
}
