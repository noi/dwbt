import { ReactNode, useEffect, useRef, useState } from 'react'

/** A button that toggles a floating panel, closed by clicking outside. */
export function Popover(props: {
  label: ReactNode
  title?: string
  className?: string
  align?: 'left' | 'right'
  children: (close: () => void) => ReactNode
}) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false)
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])
  return (
    <div className="popover" ref={ref}>
      <button type="button" className={props.className} title={props.title} onClick={() => setOpen(!open)}>
        {props.label}
      </button>
      {open && <div className={`popover-panel ${props.align ?? 'left'}`}>{props.children(() => setOpen(false))}</div>}
    </div>
  )
}
