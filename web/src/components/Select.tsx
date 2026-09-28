import { Children, isValidElement, useEffect, useId, useLayoutEffect, useRef, useState, type ButtonHTMLAttributes, type ComponentProps, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import './controls.css'

// A select-only combobox: focus stays on the trigger while the listbox is open.
// Its popup is portalled so cards, dialogs and the sidebar never clip options.
export function Select({ children, value, onChange, className = '', display, ...props }: Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'onChange' | 'value' | 'children'> & {
  children: ReactNode
  value: string | number
  onChange: (value: string) => void
  display?: ReactNode
}) {
  const options = Children.toArray(children).filter(isValidElement<ComponentProps<'option'>>).map(option => ({
    value: String(option.props.value ?? option.props.children),
    label: option.props.children,
    disabled: Boolean(option.props.disabled),
  }))
  const selected = options.findIndex(option => option.value === String(value))
  const [open, setOpen] = useState(false)
  const [active, setActive] = useState(0)
  const [position, setPosition] = useState({ left: 0, top: 0, width: 0, maxHeight: 280 })
  const trigger = useRef<HTMLButtonElement>(null)
  const list = useRef<HTMLDivElement>(null)
  const search = useRef({ text: '', time: 0 })
  const id = useId()

  const show = () => {
    // WebKit does not focus buttons on a pointer click by default.
    trigger.current?.focus({ preventScroll: true })
    setActive(selected >= 0 && !options[selected].disabled ? selected : options.findIndex(option => !option.disabled))
    setOpen(true)
  }
  const choose = (index: number) => {
    const option = options[index]
    if (!option || option.disabled) return
    setOpen(false)
    if (option.value !== String(value)) onChange(option.value)
    trigger.current?.focus()
  }

  useLayoutEffect(() => {
    if (!open) return
    const place = () => {
      const rect = trigger.current!.getBoundingClientRect()
      const width = Math.min(Math.max(rect.width, 180), window.innerWidth - 16)
      const below = window.innerHeight - rect.bottom - 12
      const above = rect.top - 12
      const up = below < Math.min(options.length * 34 + 8, 280) && above > below
      const maxHeight = Math.max(40, Math.min(280, up ? above : below))
      const height = Math.min(list.current?.scrollHeight ?? maxHeight, maxHeight)
      setPosition({ left: Math.max(8, Math.min(rect.left, window.innerWidth - width - 8)), top: up ? rect.top - height - 6 : rect.bottom + 6, width, maxHeight })
    }
    place()
    const outside = (event: PointerEvent) => {
      if (!trigger.current?.contains(event.target as Node) && !list.current?.contains(event.target as Node)) setOpen(false)
    }
    const scroll = (event: Event) => { if (!list.current?.contains(event.target as Node)) setOpen(false) }
    const close = () => setOpen(false)
    document.addEventListener('pointerdown', outside)
    document.addEventListener('scroll', scroll, true)
    window.addEventListener('resize', close)
    window.addEventListener('blur', close)
    return () => {
      document.removeEventListener('pointerdown', outside)
      document.removeEventListener('scroll', scroll, true)
      window.removeEventListener('resize', close)
      window.removeEventListener('blur', close)
    }
  }, [open, options.length])

  useEffect(() => { if (open) document.getElementById(`${id}-${active}`)?.scrollIntoView?.({ block: 'nearest' }) }, [active, id, open])
  useEffect(() => { if (props.disabled) setOpen(false) }, [props.disabled])

  return <>
    <button {...props} ref={trigger} type="button" role="combobox" value={value}
      className={`select-control ${className}`} aria-expanded={open} aria-haspopup="listbox"
      aria-controls={open ? id : undefined} aria-activedescendant={open && active >= 0 ? `${id}-${active}` : undefined}
      onClick={() => open ? setOpen(false) : show()} onBlur={() => setOpen(false)}
      onKeyDown={event => {
        if (event.key === 'Tab') { setOpen(false); return }
        if (event.key === 'Escape') { if (open) { event.preventDefault(); event.stopPropagation(); setOpen(false) } return }
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault(); if (open) choose(active); else show(); return
        }
        if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
          event.preventDefault()
          if (!open) { show(); return }
          const enabled = options.map((option, index) => option.disabled ? -1 : index).filter(index => index >= 0)
          if (!enabled.length) return
          const current = enabled.indexOf(active)
          setActive(event.key === 'Home' ? enabled[0] : event.key === 'End' ? enabled.at(-1)! : enabled[(current + (event.key === 'ArrowDown' ? 1 : enabled.length - 1)) % enabled.length])
        } else if (event.key.length === 1 && !event.metaKey && !event.ctrlKey && !event.altKey) {
          event.preventDefault()
          const now = Date.now()
          search.current = { text: (now - search.current.time < 700 ? search.current.text : '') + event.key.toLocaleLowerCase(), time: now }
          const index = options.findIndex(option => !option.disabled && Children.toArray(option.label).join('').toLocaleLowerCase().startsWith(search.current.text))
          if (index >= 0) { setOpen(true); setActive(index) }
        }
      }}>
      {display ?? <><span className="select-value">{options[selected]?.label ?? '\u00a0'}</span><span className="select-chevron" aria-hidden="true">⌄</span></>}
    </button>
    {open && createPortal(<div ref={list} id={id} role="listbox" aria-label={props['aria-label']} className="select-popup" style={position}>
      {options.map((option, index) => <div key={option.value} id={`${id}-${index}`} role="option" aria-selected={index === selected}
        aria-disabled={option.disabled || undefined} data-value={option.value} className={`select-option ${index === active ? 'active' : ''}`}
        onPointerMove={() => { if (!option.disabled) setActive(index) }} onPointerDown={event => event.preventDefault()}
        onClick={event => { event.stopPropagation(); choose(index) }}>
        <span className="select-check" aria-hidden="true">{index === selected ? '✓' : ''}</span><span>{option.label}</span>
      </div>)}
    </div>, document.body)}
  </>
}
