// Prototype-local Spinner — pure presentational, no shared imports.
interface SpinnerProps {
  className?: string;
}

export default function Spinner({ className = "" }: SpinnerProps) {
  return (
    <div className={`flex items-center justify-center ${className}`}>
      <div className="h-6 w-6 animate-spin rounded-full border-2 border-surface-300 border-t-primary" />
    </div>
  );
}
