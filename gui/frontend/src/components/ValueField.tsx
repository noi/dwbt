import { useLayoutEffect, useRef } from 'react'
import { Ref, RefGroup } from '../refs'
import { Popover } from './Popover'

/**
 * A text field for a YAML value or an expression, with a menu of the values
 * it usually takes and of the references available at that place.
 */
export function ValueField(props: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  /** Values the field usually takes; picking one replaces the value. */
  choices?: string[]
  refs?: RefGroup[]
  /**
   * Called when a reference is picked, with a function computing the new
   * value of the field for the expression to insert. Defaults to inserting
   * ref.expr with onChange.
   */
  onPick?: (ref: Ref, insert: (expr: string) => string) => void
  /** Inserts bare expressions instead of templates such as <<expr>>. */
  expression?: boolean
  invalid?: boolean
}) {
  const area = useRef<HTMLTextAreaElement>(null)
  // Whether the field has been focused, so that its selection is meaningful.
  const focused = useRef(false)
  useLayoutEffect(() => {
    const el = area.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${el.scrollHeight}px`
  }, [props.value])

  const insert = (expr: string) => {
    const el = area.current
    const text = props.expression ? expr : `<<${expr}>>`
    const start = el && focused.current ? el.selectionStart : props.value.length
    const end = el && focused.current ? el.selectionEnd : props.value.length
    return props.value.slice(0, start) + text + props.value.slice(end)
  }
  const pick = (ref: Ref) => {
    if (props.onPick) props.onPick(ref, insert)
    else props.onChange(insert(ref.expr))
    requestAnimationFrame(() => area.current?.focus())
  }

  const choices = props.choices ?? []
  const groups = props.refs ?? []
  return (
    <div className={`value-field${props.invalid ? ' invalid' : ''}`}>
      <textarea
        ref={area}
        rows={1}
        spellCheck={false}
        value={props.value}
        placeholder={props.placeholder}
        onChange={(e) => props.onChange(e.target.value)}
        onFocus={() => (focused.current = true)}
      />
      {(choices.length > 0 || groups.length > 0) && (
        <Popover label="{}" title="候補・参照を挿入" className="ref-button" align="right">
          {(close) => (
            <div className="ref-menu">
              {choices.length > 0 && (
                <div className="ref-group">
                  <div className="ref-group-label">候補</div>
                  {choices.map((c) => (
                    <button
                      type="button"
                      key={c}
                      className={c === props.value ? 'selected' : undefined}
                      onClick={() => {
                        close()
                        props.onChange(c)
                      }}
                    >
                      <code>{c}</code>
                    </button>
                  ))}
                </div>
              )}
              {groups.map((g) => (
                <div key={g.label} className="ref-group">
                  <div className="ref-group-label">{g.label}</div>
                  {g.refs.map((r) => (
                    <button
                      type="button"
                      key={r.expr}
                      onClick={() => {
                        close()
                        pick(r)
                      }}
                    >
                      <code>{r.expr}</code>
                      {r.detail && <span className="muted">{r.detail}</span>}
                    </button>
                  ))}
                </div>
              ))}
            </div>
          )}
        </Popover>
      )}
    </div>
  )
}
