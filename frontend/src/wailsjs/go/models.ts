export namespace types {
	
	export class AgentInfo {
	    name: string;
	    binary: string;
	    path: string;
	    installed: boolean;
	    version?: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.binary = source["binary"];
	        this.path = source["path"];
	        this.installed = source["installed"];
	        this.version = source["version"];
	    }
	}
	export class BranchMapping {
	    localBranch: string;
	    remoteBranch: string;
	
	    static createFrom(source: any = {}) {
	        return new BranchMapping(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.localBranch = source["localBranch"];
	        this.remoteBranch = source["remoteBranch"];
	    }
	}
	export class PostSyncCommand {
	    id: string;
	    name: string;
	    cmd: string;
	
	    static createFrom(source: any = {}) {
	        return new PostSyncCommand(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.cmd = source["cmd"];
	    }
	}
	export class Time {
	
	
	    static createFrom(source: any = {}) {
	        return new Time(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}
	export class WorkflowStepRecord {
	    step: string;
	    status: string;
	    startedAt?: Time;
	    endedAt?: Time;
	    message?: string;
	    error?: string;
	    resolveSessionId?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkflowStepRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.step = source["step"];
	        this.status = source["status"];
	        this.startedAt = this.convertValues(source["startedAt"], Time);
	        this.endedAt = this.convertValues(source["endedAt"], Time);
	        this.message = source["message"];
	        this.error = source["error"];
	        this.resolveSessionId = source["resolveSessionId"];
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
	export class SyncWorkflow {
	    runId: string;
	    steps: WorkflowStepRecord[];
	    status: string;
	    // Go type: time
	    startedAt: any;
	    finishedAt?: Time;
	    oldHEAD?: string;
	
	    static createFrom(source: any = {}) {
	        return new SyncWorkflow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runId = source["runId"];
	        this.steps = this.convertValues(source["steps"], WorkflowStepRecord);
	        this.status = source["status"];
	        this.startedAt = this.convertValues(source["startedAt"], null);
	        this.finishedAt = this.convertValues(source["finishedAt"], Time);
	        this.oldHEAD = source["oldHEAD"];
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
	export class Repo {
	    id: string;
	    name: string;
	    path: string;
	    origin: string;
	    upstream: string;
	    branch: string;
	    branchMapping?: BranchMapping;
	    autoSync: boolean;
	    syncInterval: string;
	    postSyncCommands?: PostSyncCommand[];
	    workflow?: SyncWorkflow;
	    // Go type: time
	    createdAt: any;
	    lastSync?: Time;
	    status: string;
	    aheadBy: number;
	    behindBy: number;
	    errorMessage?: string;
	
	    static createFrom(source: any = {}) {
	        return new Repo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.origin = source["origin"];
	        this.upstream = source["upstream"];
	        this.branch = source["branch"];
	        this.branchMapping = this.convertValues(source["branchMapping"], BranchMapping);
	        this.autoSync = source["autoSync"];
	        this.syncInterval = source["syncInterval"];
	        this.postSyncCommands = this.convertValues(source["postSyncCommands"], PostSyncCommand);
	        this.workflow = this.convertValues(source["workflow"], SyncWorkflow);
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.lastSync = this.convertValues(source["lastSync"], Time);
	        this.status = source["status"];
	        this.aheadBy = source["aheadBy"];
	        this.behindBy = source["behindBy"];
	        this.errorMessage = source["errorMessage"];
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
	export class StatusData {
	    repos: Repo[];
	    agents: AgentInfo[];
	    preferredAgent: string;
	
	    static createFrom(source: any = {}) {
	        return new StatusData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repos = this.convertValues(source["repos"], Repo);
	        this.agents = this.convertValues(source["agents"], AgentInfo);
	        this.preferredAgent = source["preferredAgent"];
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

