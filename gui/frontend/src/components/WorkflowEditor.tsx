import { useState } from 'react'
import { ActionInfo, emptyAction, Problem, StepResult, Workflow } from '../model'
import { ActionEditor } from './ActionEditor'
import { StepList } from './StepList'

/** Edits a workflow: its local actions and steps. */
export function WorkflowEditor(props: {
  workflow: Workflow
  onChange: (wf: Workflow) => void
  actions: Map<string, ActionInfo>
  servers: string[]
  problems: Map<number, Problem[]>
  results?: Map<number, StepResult>
}) {
  const { workflow: wf } = props
  const [openActions, setOpenActions] = useState(false)
  const set = (patch: Partial<Workflow>) => props.onChange({ ...wf, ...patch })

  return (
    <div className="workflow-editor">
      <details className="local-actions" open={openActions} onToggle={(e) => setOpenActions(e.currentTarget.open)}>
        <summary>
          このワークフロー内のアクション <span className="count">{wf.actions.length}</span>
        </summary>
        {wf.actions.map((a, i) => (
          <div className="local-action" key={i}>
            <div className="row">
              <input
                className="action-name"
                value={a.name}
                placeholder="local/name"
                spellCheck={false}
                onChange={(e) => set({ actions: wf.actions.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)) })}
              />
              <input
                className="grow"
                value={a.description}
                placeholder="説明"
                onChange={(e) =>
                  set({ actions: wf.actions.map((x, j) => (j === i ? { ...x, description: e.target.value } : x)) })
                }
              />
              <button
                type="button"
                className="icon danger"
                title="削除"
                onClick={() => set({ actions: wf.actions.filter((_, j) => j !== i) })}
              >
                ×
              </button>
            </div>
            <ActionEditor
              action={a}
              onChange={(n) => set({ actions: wf.actions.map((x, j) => (j === i ? n : x)) })}
              actions={props.actions}
              servers={props.servers}
            />
          </div>
        ))}
        <button type="button" className="add" onClick={() => set({ actions: [...wf.actions, emptyAction()] })}>
          + アクションを定義
        </button>
      </details>

      <StepList
        steps={wf.steps}
        onChange={(steps) => set({ steps })}
        scope={{ actions: props.actions }}
        servers={props.servers}
        problems={props.problems}
        results={props.results}
      />
    </div>
  )
}
