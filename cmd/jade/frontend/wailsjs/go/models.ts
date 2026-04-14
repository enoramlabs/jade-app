export namespace core {
	
	export class Note {
	    ID: string;
	    Title: string;
	    Tags: string[];
	    Frontmatter: Record<string, any>;
	    // Go type: time
	    ModTime: any;
	    Snippet: string;
	    Body: string;
	    ETag: string;
	
	    static createFrom(source: any = {}) {
	        return new Note(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Title = source["Title"];
	        this.Tags = source["Tags"];
	        this.Frontmatter = source["Frontmatter"];
	        this.ModTime = this.convertValues(source["ModTime"], null);
	        this.Snippet = source["Snippet"];
	        this.Body = source["Body"];
	        this.ETag = source["ETag"];
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
	export class NoteMeta {
	    ID: string;
	    Title: string;
	    Tags: string[];
	    Frontmatter: Record<string, any>;
	    // Go type: time
	    ModTime: any;
	    Snippet: string;
	
	    static createFrom(source: any = {}) {
	        return new NoteMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Title = source["Title"];
	        this.Tags = source["Tags"];
	        this.Frontmatter = source["Frontmatter"];
	        this.ModTime = this.convertValues(source["ModTime"], null);
	        this.Snippet = source["Snippet"];
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
	
	export class StartupState {
	    vaultPath: string;
	    vaultError: string;
	    recent: string[];
	
	    static createFrom(source: any = {}) {
	        return new StartupState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.vaultPath = source["vaultPath"];
	        this.vaultError = source["vaultError"];
	        this.recent = source["recent"];
	    }
	}
	export class VaultInfo {
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new VaultInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	    }
	}

}

