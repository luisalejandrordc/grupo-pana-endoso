import { Component, type ErrorInfo, type ReactNode } from "react";
import { ErrorState } from "@endosoflow/ui";

interface Props {
  children: ReactNode;
  /** When this changes (the address, say) a failed screen gets another chance. */
  resetKey?: string;
}

/** Keeps a bug in one screen from taking the whole app down: the shell stays and the screen explains. */
export class ErrorBoundary extends Component<Props, { failed: boolean }> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("Screen failed to render", error, info.componentStack);
  }

  componentDidUpdate(previous: Props) {
    if (this.state.failed && previous.resetKey !== this.props.resetKey)
      this.setState({ failed: false });
  }

  render() {
    if (!this.state.failed) return this.props.children;
    return (
      <ErrorState
        page
        title="Algo salió mal en esta pantalla"
        message="Recarga la página. Si el problema continúa, avisa al administrador."
        onRetry={() => window.location.reload()}
      />
    );
  }
}
