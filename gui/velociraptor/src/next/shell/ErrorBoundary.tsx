import React from 'react';
import { TriangleAlert } from 'lucide-react';
import { withRouter, type RouteComponentProps } from 'react-router-dom';
import { Button, EmptyState } from '../ui/primitives';

interface State {
  error: Error | null;
}

class Boundary extends React.Component<React.PropsWithChildren<RouteComponentProps>, State> {
  state: State = { error: null };
  private unlisten?: () => void;

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidMount() {
    // Navigating away clears the error.
    this.unlisten = this.props.history.listen(() => {
      if (this.state.error) this.setState({ error: null });
    });
  }

  componentWillUnmount() {
    this.unlisten?.();
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    console.error('UI error', error, info.componentStack);
  }

  render() {
    if (this.state.error) {
      return (
        <EmptyState icon={<TriangleAlert />} title="Something went wrong on this page"
          action={<Button onClick={() => this.setState({ error: null })}>Try again</Button>}>
          <span className="tw:font-mono tw:text-2xs">{this.state.error.message}</span>
        </EmptyState>
      );
    }
    return this.props.children;
  }
}

export const ErrorBoundary = withRouter(Boundary);
