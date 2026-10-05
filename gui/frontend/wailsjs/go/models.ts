export namespace doc {
	
	export class Expect {
	    status?: number;
	    asserts: string[];
	
	    static createFrom(source: any = {}) {
	        return new Expect(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.asserts = source["asserts"];
	    }
	}
	export class Binding {
	    name: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new Binding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.value = source["value"];
	    }
	}
	export class Step {
	    id: string;
	    use: string;
	    inputs: Binding[];
	    foreach: string;
	    params: Binding[];
	    expects: Expect[];
	    outputs: Binding[];
	
	    static createFrom(source: any = {}) {
	        return new Step(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.use = source["use"];
	        this.inputs = this.convertValues(source["inputs"], Binding);
	        this.foreach = source["foreach"];
	        this.params = this.convertValues(source["params"], Binding);
	        this.expects = this.convertValues(source["expects"], Expect);
	        this.outputs = this.convertValues(source["outputs"], Binding);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Param {
	    name: string;
	    type: string;
	
	    static createFrom(source: any = {}) {
	        return new Param(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	    }
	}
	export class Action {
	    name: string;
	    description: string;
	    params: Param[];
	    steps: Step[];
	    outputs: Binding[];
	
	    static createFrom(source: any = {}) {
	        return new Action(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.params = this.convertValues(source["params"], Param);
	        this.steps = this.convertValues(source["steps"], Step);
	        this.outputs = this.convertValues(source["outputs"], Binding);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	export class Workflow {
	    description: string;
	    actions: Action[];
	    steps: Step[];
	
	    static createFrom(source: any = {}) {
	        return new Workflow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.description = source["description"];
	        this.actions = this.convertValues(source["actions"], Action);
	        this.steps = this.convertValues(source["steps"], Step);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace studio {
	
	export class ParamInfo {
	    name: string;
	    type: string;
	    required: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ParamInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.required = source["required"];
	    }
	}
	export class ActionInfo {
	    name: string;
	    description: string;
	    builtin: boolean;
	    kind: string;
	    params: ParamInfo[];
	    outputs: string[];
	
	    static createFrom(source: any = {}) {
	        return new ActionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.builtin = source["builtin"];
	        this.kind = source["kind"];
	        this.params = this.convertValues(source["params"], ParamInfo);
	        this.outputs = source["outputs"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class Problem {
	    message: string;
	    file: string;
	    line: number;
	    step: number;
	
	    static createFrom(source: any = {}) {
	        return new Problem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message = source["message"];
	        this.file = source["file"];
	        this.line = source["line"];
	        this.step = source["step"];
	    }
	}
	export class Project {
	    root: string;
	    workflows: string[];
	    actions: ActionInfo[];
	    environments: string[];
	    defaultEnv: string;
	    servers: string[];
	    warnings: string[];
	    problems: Problem[];
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = source["root"];
	        this.workflows = source["workflows"];
	        this.actions = this.convertValues(source["actions"], ActionInfo);
	        this.environments = source["environments"];
	        this.defaultEnv = source["defaultEnv"];
	        this.servers = source["servers"];
	        this.warnings = source["warnings"];
	        this.problems = this.convertValues(source["problems"], Problem);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RunOptions {
	    env: string;
	    servers: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new RunOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.env = source["env"];
	        this.servers = source["servers"];
	    }
	}
	export class StepResult {
	    index: number;
	    label: string;
	    status: string;
	    duration: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new StepResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.label = source["label"];
	        this.status = source["status"];
	        this.duration = source["duration"];
	        this.error = source["error"];
	    }
	}
	export class RunResult {
	    status: string;
	    duration: string;
	    steps: StepResult[];
	    problems: Problem[];
	
	    static createFrom(source: any = {}) {
	        return new RunResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.duration = source["duration"];
	        this.steps = this.convertValues(source["steps"], StepResult);
	        this.problems = this.convertValues(source["problems"], Problem);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

