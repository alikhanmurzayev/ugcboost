import { type ReactNode } from "react";
import type { DictionaryItem } from "@/api/dictionaries";

export default function CategoryChip({
  children,
  testid,
}: {
  children: ReactNode;
  testid?: string;
}) {
  return (
    <span
      className="inline-flex items-center rounded-full border border-surface-300 bg-surface-100 px-2.5 py-0.5 text-xs text-gray-800"
      data-testid={testid}
    >
      {children}
    </span>
  );
}

export function CategoryChips({ categories }: { categories: DictionaryItem[] }) {
  if (categories.length === 0) return <span className="text-gray-400">—</span>;
  return (
    <div className="flex flex-wrap gap-1">
      {categories.map((c) => (
        <CategoryChip key={c.code}>{c.name}</CategoryChip>
      ))}
    </div>
  );
}
