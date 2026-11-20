// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { Component, type ErrorInfo, type ReactNode } from 'react';
import { Icon } from './Icon';

interface Props {
  /** Changing this resets the boundary, so switching tabs clears a previous crash. */
  resetKey?: string;
  label?: string;
  children: ReactNode;
}

interface State {
  error: Error | null;
}

/**
 * Catches a render error in one panel instead of letting it unmount the whole application.
 * Without it a single bad value — an API field the UI expected and the server did not send —
 * takes the entire UI down to a blank page.
 */
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('panel crashed:', error, info.componentStack);
  }

  componentDidUpdate(prev: Props) {
    if (prev.resetKey !== this.props.resetKey && this.state.error) {
      this.setState({ error: null });
    }
  }

  render() {
    const { error } = this.state;
    if (!error) {
      return this.props.children;
    }
    return (
      <div className="tab-scroll">
        <div className="err-box" role="alert">
          <b>
            <Icon name="warning" size={14} /> {this.props.label ?? 'This panel'} could not be displayed.
          </b>
          <div className="xs mono" style={{ marginTop: 6, whiteSpace: 'pre-wrap' }}>
            {error.message}
          </div>
          <div className="xs muted" style={{ marginTop: 6 }}>
            The rest of the application still works. The browser console holds the component stack.
          </div>
          <div style={{ marginTop: 8 }}>
            <button className="btn sm" onClick={() => this.setState({ error: null })}>
              Try again
            </button>
          </div>
        </div>
      </div>
    );
  }
}
