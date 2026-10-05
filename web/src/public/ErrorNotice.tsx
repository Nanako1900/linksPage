export function ErrorNotice({ message }: { message: string }) {
  return (
    <main className="mx-auto max-w-[34rem] px-5 pt-12">
      <p role="alert" className="rounded-card border border-border bg-surface px-5 py-4 text-sm">
        {message}
      </p>
    </main>
  );
}
