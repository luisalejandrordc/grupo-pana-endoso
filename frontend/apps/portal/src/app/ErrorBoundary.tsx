import { ErrorState } from "@endosoflow/ui";
import { Component, type ErrorInfo, type ReactNode } from "react";

/** A crash in one screen leaves the shell standing and offers a way back. */
export class ErrorBoundary extends Component<
  { children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("Portal screen failed", error, info.componentStack);
  }

  render() {
    if (!this.state.failed) return this.props.children;
    return (
      <ErrorState
        page
        title="Algo salió mal"
        message="Esta pantalla no pudo mostrarse. Recargue la página; si el problema continúa, comuníquese con su asesor."
        onRetry={() => window.location.reload()}
      />
    );
  }
}
