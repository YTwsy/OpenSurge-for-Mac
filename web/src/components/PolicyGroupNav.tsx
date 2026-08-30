import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { t } from '../i18n'

type PolicyGroupNavProps = {
  groups: string[]
  activeGroup: string | null
  onNavigate: (group: string) => void
  displayName?: (group: string) => string
}

function prefersReducedMotion() {
  return typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

const identityGroupName = (group: string) => group

export function PolicyGroupNav({ groups, activeGroup, onNavigate, displayName = identityGroupName }: PolicyGroupNavProps) {
  const scrollerRef = useRef<HTMLElement | null>(null)
  const buttonRefs = useRef(new Map<string, HTMLButtonElement>())
  const [canScrollLeft, setCanScrollLeft] = useState(false)
  const [canScrollRight, setCanScrollRight] = useState(false)

  const updateScrollEdges = useCallback(() => {
    const scroller = scrollerRef.current
    if (!scroller) return
    const maxScroll = Math.max(0, scroller.scrollWidth - scroller.clientWidth)
    setCanScrollLeft(scroller.scrollLeft > 2)
    setCanScrollRight(scroller.scrollLeft < maxScroll - 2)
  }, [])

  useEffect(() => {
    updateScrollEdges()
    const scroller = scrollerRef.current
    const resizeObserver = typeof ResizeObserver === 'undefined' || !scroller
      ? null
      : new ResizeObserver(updateScrollEdges)
    if (scroller) resizeObserver?.observe(scroller)
    window.addEventListener('resize', updateScrollEdges)
    return () => {
      resizeObserver?.disconnect()
      window.removeEventListener('resize', updateScrollEdges)
    }
  }, [groups, updateScrollEdges])

  useEffect(() => {
    if (!activeGroup) return
    const scroller = scrollerRef.current
    const button = buttonRefs.current.get(activeGroup)
    if (!scroller || !button) return
    const left = button.offsetLeft - (scroller.clientWidth - button.offsetWidth) / 2
    scroller.scrollTo?.({
      left: Math.max(0, left),
      behavior: prefersReducedMotion() ? 'auto' : 'smooth',
    })
  }, [activeGroup])

  const scrollNavigation = (direction: -1 | 1) => {
    const scroller = scrollerRef.current
    if (!scroller) return
    scroller.scrollBy?.({
      left: direction * Math.max(180, scroller.clientWidth * 0.72),
      behavior: prefersReducedMotion() ? 'auto' : 'smooth',
    })
  }

  const handleWheel = useCallback((event: WheelEvent) => {
    const scroller = scrollerRef.current
    if (!scroller || Math.abs(event.deltaX) > Math.abs(event.deltaY) || event.deltaY === 0) return
    const maxScroll = Math.max(0, scroller.scrollWidth - scroller.clientWidth)
    const canMove = event.deltaY < 0 ? scroller.scrollLeft > 0 : scroller.scrollLeft < maxScroll
    if (!canMove) return
    event.preventDefault()
    const multiplier = event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? scroller.clientWidth : 1
    scroller.scrollLeft = Math.min(maxScroll, Math.max(0, scroller.scrollLeft + event.deltaY * multiplier))
    updateScrollEdges()
  }, [updateScrollEdges])

  useEffect(() => {
    const scroller = scrollerRef.current
    if (!scroller) return
    scroller.addEventListener('wheel', handleWheel, { passive: false })
    return () => scroller.removeEventListener('wheel', handleWheel)
  }, [groups, handleWheel])

  const handleKeyDown = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    let nextIndex = index
    if (event.key === 'ArrowLeft') nextIndex = Math.max(0, index - 1)
    else if (event.key === 'ArrowRight') nextIndex = Math.min(groups.length - 1, index + 1)
    else if (event.key === 'Home') nextIndex = 0
    else if (event.key === 'End') nextIndex = groups.length - 1
    else return
    event.preventDefault()
    const nextGroup = groups[nextIndex]
    buttonRefs.current.get(nextGroup)?.focus()
    onNavigate(nextGroup)
  }

  if (!groups.length) return null

  return <div className="policy-group-nav-shell">
    <button className="policy-group-nav-step" type="button" aria-label={t('向左浏览策略组')} disabled={!canScrollLeft} onClick={() => scrollNavigation(-1)}><span aria-hidden="true">‹</span></button>
    <div className={`policy-group-nav-viewport ${canScrollLeft ? 'can-scroll-left' : ''} ${canScrollRight ? 'can-scroll-right' : ''}`}>
      <nav className="policy-group-nav" aria-label={t('策略组快速导航')} ref={scrollerRef} onScroll={updateScrollEdges}>
        {groups.map((group, index) => <button
          type="button"
          key={group}
          ref={node => { if (node) buttonRefs.current.set(group, node); else buttonRefs.current.delete(group) }}
          title={displayName(group) === group ? group : `${displayName(group)} · ${group}`}
          aria-current={group === activeGroup ? 'location' : undefined}
          tabIndex={group === activeGroup || (!activeGroup && index === 0) ? 0 : -1}
          onClick={() => onNavigate(group)}
          onKeyDown={event => handleKeyDown(event, index)}
        >{displayName(group)}</button>)}
      </nav>
    </div>
    <button className="policy-group-nav-step" type="button" aria-label={t('向右浏览策略组')} disabled={!canScrollRight} onClick={() => scrollNavigation(1)}><span aria-hidden="true">›</span></button>
  </div>
}
