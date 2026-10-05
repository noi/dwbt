// Types mirroring the Go side (gui/internal/doc and gui/internal/studio).
// The generated classes in wailsjs/go/models.ts are not used directly so that
// documents can be edited as plain immutable objects.

export interface Binding {
  name: string
  value: string
}

export interface Expect {
  status?: number | null
  asserts: string[]
}

export interface Step {
  /** Client-side key for rendering; ignored by the Go side. */
  _key?: string
  id: string
  use: string
  inputs: Binding[]
  foreach: string
  params: Binding[]
  expects: Expect[]
  outputs: Binding[]
}

export interface Param {
  name: string
  type: string
}

export interface Action {
  name: string
  description: string
  params: Param[]
  steps: Step[]
  outputs: Binding[]
}

export interface Workflow {
  description: string
  actions: Action[]
  steps: Step[]
}

export interface ParamInfo {
  name: string
  type: string
  required: boolean
}

export interface ActionInfo {
  name: string
  description: string
  builtin: boolean
  kind: string
  params: ParamInfo[]
  outputs: string[]
  /** Set for actions defined in the edited workflow file. */
  local?: boolean
}

export interface Problem {
  message: string
  file: string
  line: number
  step: number
}

export interface Project {
  root: string
  workflows: string[]
  actions: ActionInfo[]
  environments: string[]
  defaultEnv: string
  servers: string[]
  warnings: string[]
  problems: Problem[]
}

export interface StepResult {
  index: number
  label: string
  status: string
  duration: string
  error: string
}

export interface RunResult {
  status: string
  duration: string
  steps: StepResult[]
  problems: Problem[]
}

export interface RunOptions {
  env: string
  servers: Record<string, string>
}

export const paramTypes = ['string', 'number', 'bool', 'object', 'array', 'any']

let seq = 0
export function newKey(): string {
  return `k${++seq}`
}

export function emptyStep(use = ''): Step {
  return { _key: newKey(), id: '', use, inputs: [], foreach: '', params: [], expects: [], outputs: [] }
}

export function emptyAction(name = ''): Action {
  return { name, description: '', params: [], steps: [], outputs: [] }
}

export function emptyWorkflow(): Workflow {
  return { description: '', actions: [], steps: [] }
}

// The Go side encodes empty slices of some values as null; normalize them so
// that the editor can rely on arrays.

function normSteps(steps: Step[] | null): Step[] {
  return (steps ?? []).map((s) => ({
    _key: newKey(),
    id: s.id ?? '',
    use: s.use ?? '',
    inputs: s.inputs ?? [],
    foreach: s.foreach ?? '',
    params: s.params ?? [],
    expects: (s.expects ?? []).map((e) => ({ status: e.status ?? null, asserts: e.asserts ?? [] })),
    outputs: s.outputs ?? [],
  }))
}

export function normAction(a: Action): Action {
  return {
    name: a.name ?? '',
    description: a.description ?? '',
    params: a.params ?? [],
    steps: normSteps(a.steps),
    outputs: a.outputs ?? [],
  }
}

export function normWorkflow(w: Workflow): Workflow {
  return { description: w.description ?? '', actions: (w.actions ?? []).map(normAction), steps: normSteps(w.steps) }
}

export function normProject(p: Project): Project {
  return {
    ...p,
    workflows: p.workflows ?? [],
    actions: (p.actions ?? []).map((a) => ({ ...a, params: a.params ?? [], outputs: a.outputs ?? [] })),
    environments: p.environments ?? [],
    servers: p.servers ?? [],
    warnings: p.warnings ?? [],
    problems: p.problems ?? [],
  }
}

/** Describes an action of a document, for use in the palette. */
export function infoOf(a: Action, local: boolean): ActionInfo {
  return {
    name: a.name,
    description: a.description,
    builtin: false,
    kind: '',
    params: a.params.map((p) => ({ name: p.name, type: p.type, required: true })),
    outputs: a.outputs.map((o) => o.name),
    local,
  }
}
