export interface SortState {
  key: string;
  dir: "asc" | "desc";
}

interface SortableColumn<T> {
  key: string;
  sortValue?: (row: T) => string | number;
}

export function sortApplications<T>(
  rows: T[],
  sort: SortState | undefined,
  columns: SortableColumn<T>[],
): T[] {
  if (!sort) return rows;
  const col = columns.find((c) => c.key === sort.key);
  const sortValue = col?.sortValue;
  if (!sortValue) return rows;
  const dir = sort.dir;
  return [...rows].sort((a, b) => {
    const av = sortValue(a);
    const bv = sortValue(b);
    const cmp = av < bv ? -1 : av > bv ? 1 : 0;
    return dir === "asc" ? cmp : -cmp;
  });
}

export function toggleSort(prev: SortState | undefined, key: string): SortState {
  if (prev?.key !== key) return { key, dir: "desc" };
  return { key, dir: prev.dir === "asc" ? "desc" : "asc" };
}
