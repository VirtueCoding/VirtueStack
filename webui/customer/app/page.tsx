export default function Home() {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center p-24">
      <div className="text-center">
        <h1 className="text-4xl font-bold tracking-tight">
          VirtueStack Customer Portal
        </h1>
        <p className="mt-4 text-lg text-muted-foreground">
          Self-service portal for VirtueStack customers
        </p>
        <div className="mt-8 flex justify-center gap-4">
          <div className="rounded-lg bg-primary px-4 py-2 text-primary-foreground">
            Next.js 16
          </div>
          <div className="rounded-lg bg-secondary px-4 py-2 text-secondary-foreground">
            React 19
          </div>
          <div className="rounded-lg bg-secondary px-4 py-2 text-secondary-foreground">
            TypeScript 5.7
          </div>
          <div className="rounded-lg bg-secondary px-4 py-2 text-secondary-foreground">
            Tailwind CSS
          </div>
          <div className="rounded-lg bg-secondary px-4 py-2 text-secondary-foreground">
            shadcn/ui
          </div>
        </div>
      </div>
    </main>
  );
}
