import { Binding } from '../model'
import { Ref, RefGroup } from '../refs'
import { ValueField } from './ValueField'

/** Edits a list of named values, such as inputs and outputs. */
export function Bindings(props: {
  items: Binding[]
  onChange: (items: Binding[]) => void
  refs?: RefGroup[]
  onPick?: (index: number, ref: Ref, insert: (expr: string) => string) => void
  addLabel: string
  namePlaceholder?: string
}) {
  const set = (i: number, b: Binding) => props.onChange(props.items.map((x, j) => (j === i ? b : x)))
  return (
    <div className="bindings">
      {props.items.map((b, i) => (
        <div className="row" key={i}>
          <input
            className="name"
            value={b.name}
            placeholder={props.namePlaceholder ?? 'name'}
            spellCheck={false}
            onChange={(e) => set(i, { ...b, name: e.target.value })}
          />
          <ValueField
            value={b.value}
            refs={props.refs}
            onChange={(v) => set(i, { ...b, value: v })}
            onPick={props.onPick && ((ref, insert) => props.onPick!(i, ref, insert))}
          />
          <button
            type="button"
            className="icon"
            title="削除"
            onClick={() => props.onChange(props.items.filter((_, j) => j !== i))}
          >
            ×
          </button>
        </div>
      ))}
      <button type="button" className="add" onClick={() => props.onChange([...props.items, { name: '', value: '' }])}>
        + {props.addLabel}
      </button>
    </div>
  )
}
