import { useState } from 'react'
import { ActionInfo } from '../model'

/** Lists the callable actions with a filter. */
export function ActionPalette(props: { actions: ActionInfo[]; onPick: (a: ActionInfo) => void }) {
  const [filter, setFilter] = useState('')
  const f = filter.trim().toLowerCase()
  const shown = props.actions.filter(
    (a) => !f || a.name.toLowerCase().includes(f) || a.description.toLowerCase().includes(f),
  )
  const groups: [string, ActionInfo[]][] = [
    ['このワークフローのアクション', shown.filter((a) => a.local)],
    ['アクション', shown.filter((a) => !a.local && !a.builtin)],
    ['組み込み', shown.filter((a) => a.builtin)],
  ]
  return (
    <div className="palette">
      <input
        autoFocus
        placeholder="アクションを検索"
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && shown.length > 0) props.onPick(shown[0])
        }}
      />
      <div className="palette-list">
        {groups
          .filter(([, list]) => list.length > 0)
          .map(([label, list]) => (
            <div key={label}>
              <div className="ref-group-label">{label}</div>
              {list.map((a) => (
                <button type="button" key={a.name} onClick={() => props.onPick(a)}>
                  <span className={`action-chip${a.builtin ? ' builtin' : ''}`}>{a.name}</span>
                  <span className="muted">{a.description || signature(a)}</span>
                </button>
              ))}
            </div>
          ))}
        {shown.length === 0 && <div className="muted pad">見つかりません</div>}
      </div>
    </div>
  )
}

function signature(a: ActionInfo): string {
  const params = a.params.map((p) => p.name + (p.required ? '' : '?')).join(', ')
  const outputs = a.outputs.join(', ')
  return `(${params})${outputs ? ` → ${outputs}` : ''}`
}
