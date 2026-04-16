/**
 * Return `next` only if it differs from `prev` (by sorted element comparison).
 * Otherwise return `prev` to preserve referential equality.
 *
 * Both arrays are compared after sorting, so order doesn't matter.
 */
export function stableSortedArray(prev: string[], next: string[]): string[] {
  const sorted = next.slice().sort();
  const prevSorted = prev.slice().sort();
  if (
    prevSorted.length === sorted.length &&
    prevSorted.every((n, i) => n === sorted[i])
  ) {
    return prev;
  }
  return sorted;
}
