import { Component } from "react";
import type { ErrorInfo, ReactNode } from "react";

interface Props {
  children: ReactNode;
}

interface State {
  hasError: boolean;
}

export default class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError(): State {
    return { hasError: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("ErrorBoundary caught:", error, info.componentStack);
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="flex min-h-screen flex-col items-center justify-center bg-surface-100">
          <p className="text-lg font-medium text-gray-900">
            Что-то пошло не так
          </p>
          <p className="mt-1 text-sm text-gray-500">
            Произошла непредвиденная ошибка
          </p>
          <button
            onClick={() => {
              this.setState({ hasError: false });
              window.location.reload();
            }}
            className="mt-4 rounded-button bg-primary px-4 py-2 text-sm font-medium text-white hover:bg-primary/90"
          >
            Перезагрузить
          </button>
        </div>
      );
    }

    return this.props.children;
  }
}
