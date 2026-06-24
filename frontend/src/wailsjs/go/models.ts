export namespace agent {
	
	export class StreamEvent {
	    t: string;
	    d?: string;
	    agent?: string;
	    files?: string[];
	    // Go type: time
	    ts: any;
	    success?: boolean;
	    summary?: string;
	    session_id?: string;
	    resolvedFiles?: string[];
	    diff?: string;
	    agentName?: string;
	    name?: string;
	    path?: string;
	
	    static createFrom(source: any = {}) {
	        return new StreamEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.t = source["t"];
	        this.d = source["d"];
	        this.agent = source["agent"];
	        this.files = source["files"];
	        this.ts = this.convertValues(source["ts"], null);
	        this.success = source["success"];
	        this.summary = source["summary"];
	        this.session_id = source["session_id"];
	        this.resolvedFiles = source["resolvedFiles"];
	        this.diff = source["diff"];
	        this.agentName = source["agentName"];
	        this.name = source["name"];
	        this.path = source["path"];
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

export namespace config {
	
	export class AgentConfig {
	    Preferred: string;
	    Priority: string[];
	    Timeout: string;
	    ConflictStrategy: string;
	    ResolveStrategy: string;
	    ConfirmBeforeCommit: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AgentConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Preferred = source["Preferred"];
	        this.Priority = source["Priority"];
	        this.Timeout = source["Timeout"];
	        this.ConflictStrategy = source["ConflictStrategy"];
	        this.ResolveStrategy = source["ResolveStrategy"];
	        this.ConfirmBeforeCommit = source["ConfirmBeforeCommit"];
	    }
	}
	export class ProxyConfig {
	    Enabled: boolean;
	    URL: string;
	
	    static createFrom(source: any = {}) {
	        return new ProxyConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Enabled = source["Enabled"];
	        this.URL = source["URL"];
	    }
	}
	export class NotificationConfig {
	    Enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new NotificationConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Enabled = source["Enabled"];
	    }
	}
	export class GitHubConfig {
	    Token: string;
	
	    static createFrom(source: any = {}) {
	        return new GitHubConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Token = source["Token"];
	    }
	}
	export class SyncConfig {
	    DefaultInterval: string;
	    SyncOnStartup: boolean;
	    AutoLaunch: boolean;
	    AutoSummary: boolean;
	    SummaryAgent: string;
	    SummaryLanguage: string;
	    SummaryTimeout: string;
	
	    static createFrom(source: any = {}) {
	        return new SyncConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.DefaultInterval = source["DefaultInterval"];
	        this.SyncOnStartup = source["SyncOnStartup"];
	        this.AutoLaunch = source["AutoLaunch"];
	        this.AutoSummary = source["AutoSummary"];
	        this.SummaryAgent = source["SummaryAgent"];
	        this.SummaryLanguage = source["SummaryLanguage"];
	        this.SummaryTimeout = source["SummaryTimeout"];
	    }
	}
	export class Config {
	    Sync: SyncConfig;
	    Agent: AgentConfig;
	    GitHub: GitHubConfig;
	    Notification: NotificationConfig;
	    Proxy: ProxyConfig;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Sync = this.convertValues(source["Sync"], SyncConfig);
	        this.Agent = this.convertValues(source["Agent"], AgentConfig);
	        this.GitHub = this.convertValues(source["GitHub"], GitHubConfig);
	        this.Notification = this.convertValues(source["Notification"], NotificationConfig);
	        this.Proxy = this.convertValues(source["Proxy"], ProxyConfig);
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

export namespace main {
	
	export class AddRepoParams {
	    path: string;
	    upstream?: string;
	    branchMapping?: types.BranchMapping;
	
	    static createFrom(source: any = {}) {
	        return new AddRepoParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.upstream = source["upstream"];
	        this.branchMapping = this.convertValues(source["branchMapping"], types.BranchMapping);
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
	export class AgentCleanupResult {
	    removed: number;
	
	    static createFrom(source: any = {}) {
	        return new AgentCleanupResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.removed = source["removed"];
	    }
	}
	export class AgentLogResult {
	    events: agent.StreamEvent[];
	    isRunning: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AgentLogResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.events = this.convertValues(source["events"], agent.StreamEvent);
	        this.isRunning = source["isRunning"];
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
	export class CustomIDE {
	    id: string;
	    name: string;
	    cliCommand: string;
	
	    static createFrom(source: any = {}) {
	        return new CustomIDE(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.cliCommand = source["cliCommand"];
	    }
	}
	export class DiffResult {
	    success: boolean;
	    diff?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new DiffResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.diff = source["diff"];
	        this.error = source["error"];
	    }
	}
	export class HistoryCleanupReq {
	    repo?: string;
	    keepDays?: number;
	
	    static createFrom(source: any = {}) {
	        return new HistoryCleanupReq(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repo = source["repo"];
	        this.keepDays = source["keepDays"];
	    }
	}
	export class HistoryCleanupResult {
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new HistoryCleanupResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message = source["message"];
	    }
	}
	export class IDEInfo {
	    id: string;
	    name: string;
	    cliCommand: string;
	    appName: string;
	    installed: boolean;
	    openMethod: string;
	    isCustom?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new IDEInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.cliCommand = source["cliCommand"];
	        this.appName = source["appName"];
	        this.installed = source["installed"];
	        this.openMethod = source["openMethod"];
	        this.isCustom = source["isCustom"];
	    }
	}
	export class IDEConfig {
	    defaultIDE: string;
	    detectedIDEs: IDEInfo[];
	    customIDEs: CustomIDE[];
	
	    static createFrom(source: any = {}) {
	        return new IDEConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.defaultIDE = source["defaultIDE"];
	        this.detectedIDEs = this.convertValues(source["detectedIDEs"], IDEInfo);
	        this.customIDEs = this.convertValues(source["customIDEs"], CustomIDE);
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
	
	export class IDEOpenResult {
	    success: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new IDEOpenResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.error = source["error"];
	    }
	}
	export class PostSyncAddReq {
	    name: string;
	    cmd: string;
	
	    static createFrom(source: any = {}) {
	        return new PostSyncAddReq(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.cmd = source["cmd"];
	    }
	}
	export class PostSyncRemoveReq {
	    id: string;
	
	    static createFrom(source: any = {}) {
	        return new PostSyncRemoveReq(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	    }
	}
	export class RemoveResult {
	    removed: string;
	
	    static createFrom(source: any = {}) {
	        return new RemoveResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.removed = source["removed"];
	    }
	}
	export class RepoBranchesResult {
	    localBranches: string[];
	    remoteBranches: string[];
	
	    static createFrom(source: any = {}) {
	        return new RepoBranchesResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.localBranches = source["localBranches"];
	        this.remoteBranches = source["remoteBranches"];
	    }
	}
	export class ResolveRequest {
	    mode?: string;
	    agent?: string;
	    noConfirm?: boolean;
	    manual?: boolean;
	    retry?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ResolveRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.agent = source["agent"];
	        this.noConfirm = source["noConfirm"];
	        this.manual = source["manual"];
	        this.retry = source["retry"];
	    }
	}
	export class SetBranchMappingRequest {
	    repoName: string;
	    localBranch: string;
	    remoteBranch: string;
	
	    static createFrom(source: any = {}) {
	        return new SetBranchMappingRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repoName = source["repoName"];
	        this.localBranch = source["localBranch"];
	        this.remoteBranch = source["remoteBranch"];
	    }
	}
	export class SetBranchMappingResult {
	    success: boolean;
	    branchMapping?: types.BranchMapping;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new SetBranchMappingResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.branchMapping = this.convertValues(source["branchMapping"], types.BranchMapping);
	        this.error = source["error"];
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
	export class SummarizeReq {
	    retry?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SummarizeReq(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.retry = source["retry"];
	    }
	}
	export class SummarizeResult {
	    historyId: number;
	    repoName: string;
	    summary: string;
	    summaryStatus: string;
	
	    static createFrom(source: any = {}) {
	        return new SummarizeResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.historyId = source["historyId"];
	        this.repoName = source["repoName"];
	        this.summary = source["summary"];
	        this.summaryStatus = source["summaryStatus"];
	    }
	}

}

export namespace types {
	
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
	export class AddData {
	    repo: Repo;
	
	    static createFrom(source: any = {}) {
	        return new AddData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repo = this.convertValues(source["repo"], Repo);
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
	export class AgentListData {
	    agents: AgentInfo[];
	    preferred: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentListData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agents = this.convertValues(source["agents"], AgentInfo);
	        this.preferred = source["preferred"];
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
	export class AgentResetData {
	    repoId: string;
	    cleared: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AgentResetData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repoId = source["repoId"];
	        this.cleared = source["cleared"];
	    }
	}
	export class AgentResolveResult {
	    success: boolean;
	    resolvedFiles: string[];
	    diff: string;
	    summary: string;
	    sessionId: string;
	    agentName: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentResolveResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.resolvedFiles = source["resolvedFiles"];
	        this.diff = source["diff"];
	        this.summary = source["summary"];
	        this.sessionId = source["sessionId"];
	        this.agentName = source["agentName"];
	    }
	}
	export class AgentSessionInfo {
	    id: string;
	    repoId: string;
	    repoName: string;
	    agentName: string;
	    status: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    lastUsedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new AgentSessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.repoId = source["repoId"];
	        this.repoName = source["repoName"];
	        this.agentName = source["agentName"];
	        this.status = source["status"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.lastUsedAt = this.convertValues(source["lastUsedAt"], null);
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
	export class AgentSessionsData {
	    sessions: AgentSessionInfo[];
	
	    static createFrom(source: any = {}) {
	        return new AgentSessionsData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessions = this.convertValues(source["sessions"], AgentSessionInfo);
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
	
	export class ConflictFile {
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new ConflictFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	    }
	}
	export class SyncHistoryRecord {
	    id: number;
	    repoId: string;
	    repoName: string;
	    status: string;
	    commitsPulled: number;
	    conflictFiles: string[];
	    agentUsed: string;
	    conflictsFound: number;
	    autoResolved: number;
	    errorMessage: string;
	    summary: string;
	    summaryStatus: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new SyncHistoryRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.repoId = source["repoId"];
	        this.repoName = source["repoName"];
	        this.status = source["status"];
	        this.commitsPulled = source["commitsPulled"];
	        this.conflictFiles = source["conflictFiles"];
	        this.agentUsed = source["agentUsed"];
	        this.conflictsFound = source["conflictsFound"];
	        this.autoResolved = source["autoResolved"];
	        this.errorMessage = source["errorMessage"];
	        this.summary = source["summary"];
	        this.summaryStatus = source["summaryStatus"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class HistoryData {
	    records: SyncHistoryRecord[];
	
	    static createFrom(source: any = {}) {
	        return new HistoryData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.records = this.convertValues(source["records"], SyncHistoryRecord);
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
	
	export class PostSyncCommandsData {
	    commands: PostSyncCommand[];
	
	    static createFrom(source: any = {}) {
	        return new PostSyncCommandsData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.commands = this.convertValues(source["commands"], PostSyncCommand);
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
	export class PostSyncResult {
	    name: string;
	    cmd: string;
	    success: boolean;
	    output?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new PostSyncResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.cmd = source["cmd"];
	        this.success = source["success"];
	        this.output = source["output"];
	        this.error = source["error"];
	    }
	}
	
	export class ResolveData {
	    repoId: string;
	    conflicts: ConflictFile[];
	    agentResult?: AgentResolveResult;
	    commitError?: string;
	
	    static createFrom(source: any = {}) {
	        return new ResolveData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repoId = source["repoId"];
	        this.conflicts = this.convertValues(source["conflicts"], ConflictFile);
	        this.agentResult = this.convertValues(source["agentResult"], AgentResolveResult);
	        this.commitError = source["commitError"];
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
	export class ScannedRepo {
	    path: string;
	    name: string;
	    origin: string;
	    isFork: boolean;
	    suggestedUpstream?: string;
	    localBranches?: string[];
	    remoteBranches?: string[];
	
	    static createFrom(source: any = {}) {
	        return new ScannedRepo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.origin = source["origin"];
	        this.isFork = source["isFork"];
	        this.suggestedUpstream = source["suggestedUpstream"];
	        this.localBranches = source["localBranches"];
	        this.remoteBranches = source["remoteBranches"];
	    }
	}
	export class ScanData {
	    repos: ScannedRepo[];
	
	    static createFrom(source: any = {}) {
	        return new ScanData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repos = this.convertValues(source["repos"], ScannedRepo);
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
	export class SyncResult {
	    repoId: string;
	    repoName: string;
	    status: string;
	    commitsPulled: number;
	    conflictFiles?: string[];
	    errorMessage?: string;
	    agentUsed?: string;
	    conflictsFound?: number;
	    autoResolved?: number;
	    pendingConfirm?: string[];
	    postSyncResults?: PostSyncResult[];
	    agentResult?: AgentResolveResult;
	    commitError?: string;
	    workflow?: SyncWorkflow;
	
	    static createFrom(source: any = {}) {
	        return new SyncResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repoId = source["repoId"];
	        this.repoName = source["repoName"];
	        this.status = source["status"];
	        this.commitsPulled = source["commitsPulled"];
	        this.conflictFiles = source["conflictFiles"];
	        this.errorMessage = source["errorMessage"];
	        this.agentUsed = source["agentUsed"];
	        this.conflictsFound = source["conflictsFound"];
	        this.autoResolved = source["autoResolved"];
	        this.pendingConfirm = source["pendingConfirm"];
	        this.postSyncResults = this.convertValues(source["postSyncResults"], PostSyncResult);
	        this.agentResult = this.convertValues(source["agentResult"], AgentResolveResult);
	        this.commitError = source["commitError"];
	        this.workflow = this.convertValues(source["workflow"], SyncWorkflow);
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
	export class SyncData {
	    results: SyncResult[];
	
	    static createFrom(source: any = {}) {
	        return new SyncData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.results = this.convertValues(source["results"], SyncResult);
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

