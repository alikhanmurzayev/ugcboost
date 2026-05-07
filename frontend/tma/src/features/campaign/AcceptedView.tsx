export function AcceptedView() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-surface px-6 py-10">
      <div className="flex max-w-sm flex-col items-center text-center">
        <SuccessCheckmark />
        <p className="mt-6 text-lg leading-relaxed text-gray-900">
          Отправили вам договор на подписание по СМС, подпишите,
          пожалуйста.
        </p>
        <p className="mt-4 text-sm text-gray-500">
          Можете закрывать эту страницу.
        </p>
      </div>
    </div>
  );
}

function SuccessCheckmark() {
  return (
    <svg
      className="h-20 w-20 text-primary"
      viewBox="0 0 24 24"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
    >
      <circle cx="12" cy="12" r="11" fill="currentColor" />
      <path
        d="M7.5 12.5l2.8 2.8 6.2-6.2"
        stroke="white"
        strokeWidth="2.4"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
