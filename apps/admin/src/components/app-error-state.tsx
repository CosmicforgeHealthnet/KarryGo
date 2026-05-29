type AppErrorStateProps = {
  title?: string;
  message: string;
  actionLabel?: string;
  onAction?: () => void;
};

export function AppErrorState({
  title = "Something went wrong",
  message,
  actionLabel,
  onAction,
}: AppErrorStateProps) {
  return (
    <div className="flex min-h-[320px] w-full items-center justify-center rounded-2xl border border-red-100 bg-red-50 px-6 py-10 text-center">
      <div className="max-w-md">
        <div className="mx-auto mb-5 flex h-14 w-14 items-center justify-center rounded-full bg-white text-red-600 shadow-sm">
          <span aria-hidden="true" className="text-2xl font-semibold">
            !
          </span>
        </div>
        <h2 className="text-xl font-semibold text-zinc-950">{title}</h2>
        <p className="mt-2 text-sm leading-6 text-zinc-600">{message}</p>
        {actionLabel && onAction ? (
          <button
            type="button"
            onClick={onAction}
            className="mt-6 rounded-full bg-[#20AD4E] px-5 py-2.5 text-sm font-semibold text-white transition hover:bg-[#18893D]"
          >
            {actionLabel}
          </button>
        ) : null}
      </div>
    </div>
  );
}
