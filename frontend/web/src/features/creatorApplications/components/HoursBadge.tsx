interface Props {
  hours: number;
}

export default function HoursBadge({ hours }: Props) {
  const cls =
    hours > 48
      ? "text-red-600"
      : hours > 24
        ? "text-amber-600"
        : "text-gray-700";
  return (
    <span
      className={`font-medium ${cls}`}
      data-testid="hours-badge"
    >
      {formatHours(hours)}
    </span>
  );
}

function formatHours(hours: number): string {
  if (hours < 1) return "<1ч";
  if (hours < 24) return `${Math.round(hours)}ч`;
  const days = Math.floor(hours / 24);
  const remH = Math.round(hours - days * 24);
  return remH > 0 ? `${days}д ${remH}ч` : `${days}д`;
}
