// Typed wrappers of the methods bound by the Go App.
import * as App from '../wailsjs/go/main/App'
import {
  Action,
  normAction,
  normProject,
  normWorkflow,
  Problem,
  Project,
  RunOptions,
  RunResult,
  Workflow,
} from './model'

// The generated bindings expect the generated classes; plain objects with the
// same fields are passed instead.
const go = App as unknown as Record<string, (...args: unknown[]) => Promise<any>>

const project = async (p: Project | null) => (p ? normProject(p) : null)

export const api = {
  initial: async (): Promise<Project | null> => project(await go.Initial()),
  choose: async (): Promise<Project | null> => project(await go.Choose()),
  project: async (): Promise<Project> => normProject(await go.Project()),
  workflow: async (name: string): Promise<Workflow> => normWorkflow(await go.Workflow(name)),
  saveWorkflow: (name: string, wf: Workflow): Promise<void> => go.SaveWorkflow(name, wf),
  action: async (name: string): Promise<Action> => normAction(await go.Action(name)),
  saveAction: (name: string, a: Action): Promise<void> => go.SaveAction(name, a),
  renderWorkflow: (wf: Workflow): Promise<string> => go.RenderWorkflow(wf),
  renderAction: (a: Action): Promise<string> => go.RenderAction(a),
  checkWorkflow: async (name: string, wf: Workflow): Promise<Problem[]> => (await go.CheckWorkflow(name, wf)) ?? [],
  checkAction: async (name: string, a: Action): Promise<Problem[]> => (await go.CheckAction(name, a)) ?? [],
  run: async (name: string, wf: Workflow, opts: RunOptions): Promise<RunResult> => {
    const r: RunResult = await go.Run(name, wf, opts)
    return { ...r, steps: r.steps ?? [], problems: r.problems ?? [] }
  },
  stop: (): Promise<void> => go.Stop(),
}

/** Converts an error thrown by a bound method into a message. */
export function message(err: unknown): string {
  if (typeof err === 'string') return err
  if (err instanceof Error) return err.message
  return String(err)
}
