import { Component, type ErrorInfo, type ReactNode } from 'react'

type PageErrorBoundaryProps = {
  children: ReactNode
}

type PageErrorBoundaryState = {
  error: Error | null
}

export class PageErrorBoundary extends Component<PageErrorBoundaryProps, PageErrorBoundaryState> {
  state: PageErrorBoundaryState = { error: null }

  static getDerivedStateFromError(cause: unknown): PageErrorBoundaryState {
    return { error: cause instanceof Error ? cause : new Error(String(cause)) }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('OpenSurge page render failed', error, info)
  }

  render() {
    if (this.state.error) {
      return <section className="session-expired" role="alert">
        <span aria-hidden="true">!</span>
        <div>
          <h1>当前页面暂时无法显示</h1>
          <p>你仍可使用左侧导航打开其他页面。请重试；如果问题持续，请前往诊断页查看运行信息。</p>
          <button className="primary" type="button" onClick={() => this.setState({ error: null })}>重试当前页面</button>
        </div>
      </section>
    }
    return this.props.children
  }
}
