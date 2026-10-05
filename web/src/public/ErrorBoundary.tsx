import { Component, type ReactNode } from "react";
import { reportFailure } from "./report";

interface Props {
  fallback: ReactNode;
  children: ReactNode;
}

/** Shows `fallback` instead of unmounting the page when rendering throws. */
export class ErrorBoundary extends Component<Props, { failed: boolean }> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  componentDidCatch(error: unknown) {
    reportFailure(error);
  }

  render() {
    return this.state.failed ? this.props.fallback : this.props.children;
  }
}
