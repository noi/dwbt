import { useState } from 'react'
import { ActionInfo, emptyStep, newKey, Problem, Step, StepResult } from '../model'
import { Scope } from '../refs'
import { ActionPalette } from './ActionPalette'
import { Popover } from './Popover'
import { StepCard } from './StepCard'

/** Edits a list of steps as vertically stacked blocks. */
export function StepList(props: {
  steps: Step[]
  onChange: (steps: Step[]) => void
  scope: Omit<Scope, 'steps'>
  servers: string[]
  /** Problems by top-level step index, for the steps of the edited document. */
  problems?: Map<number, Problem[]>
  results?: Map<number, StepResult>
}) {
  const { steps } = props
  const [dragging, setDragging] = useState<number | null>(null)
  const [dropAt, setDropAt] = useState<number | null>(null)
  const scope: Scope = { ...props.scope, steps }
  const palette = [...props.scope.actions.values()]

  const set = (i: number, s: Step) => props.onChange(steps.map((x, j) => (j === i ? s : x)))
  const move = (from: number, to: number) => {
    if (to < 0 || to > steps.length || from === to || from + 1 === to) return
    const next = [...steps]
    const [s] = next.splice(from, 1)
    next.splice(to > from ? to - 1 : to, 0, s)
    props.onChange(next)
  }
  const add = (a: ActionInfo) => props.onChange([...steps, initialStep(a, props.servers)])

  return (
    <div className="step-list" onDragEnd={() => (setDragging(null), setDropAt(null))}>
      {steps.map((s, i) => (
        <div
          key={s._key ?? i}
          className={`step-slot${dropAt === i ? ' drop-before' : ''}${dropAt === i + 1 && i === steps.length - 1 ? ' drop-after' : ''}${dragging === i ? ' dragging' : ''}`}
          onDragOver={(e) => {
            if (dragging === null) return
            e.preventDefault()
            const rect = e.currentTarget.getBoundingClientRect()
            setDropAt(e.clientY < rect.top + rect.height / 2 ? i : i + 1)
          }}
          onDrop={(e) => {
            e.preventDefault()
            if (dragging !== null && dropAt !== null) move(dragging, dropAt)
            setDragging(null)
            setDropAt(null)
          }}
        >
          <StepCard
            step={s}
            index={i}
            scope={scope}
            servers={props.servers}
            palette={palette}
            problems={props.problems?.get(i) ?? []}
            result={props.results?.get(i)}
            onChange={(n) => set(i, n)}
            onRemove={() => props.onChange(steps.filter((_, j) => j !== i))}
            onDuplicate={() => {
              const next = [...steps]
              next.splice(i + 1, 0, { ...structuredClone(s), _key: newKey(), id: s.id ? `${s.id}_copy` : '' })
              props.onChange(next)
            }}
            onMove={(d) => move(i, d < 0 ? i - 1 : i + 2)}
            dragHandle={{
              draggable: true,
              onDragStart: (e) => {
                e.dataTransfer.effectAllowed = 'move'
                const card = e.currentTarget.closest('.step-slot')
                if (card) e.dataTransfer.setDragImage(card, 20, 20)
                setDragging(i)
              },
            }}
          />
        </div>
      ))}
      <Popover label="+ ステップを追加" className="add add-step">
        {(close) => (
          <ActionPalette
            actions={palette}
            onPick={(a) => {
              close()
              add(a)
            }}
          />
        )}
      </Popover>
    </div>
  )
}

/** Creates a step using a, with its required parameters. */
function initialStep(a: ActionInfo, servers: string[]): Step {
  const s = emptyStep(a.name)
  const defaults: Record<string, string> =
    a.kind === 'http' ? { server: servers[0] ?? '', method: 'GET', path: '/' } : {}
  // Parameters with defaults come first, in the order of the defaults.
  const order = Object.keys(defaults)
  const rank = (name: string) => (order.includes(name) ? order.indexOf(name) : order.length)
  s.params = a.params
    .filter((p) => p.required)
    .sort((x, y) => rank(x.name) - rank(y.name))
    .map((p) => ({ name: p.name, value: defaults[p.name] ?? '' }))
  if (a.kind === 'http') s.expects = [{ status: 200, asserts: [] }]
  return s
}
