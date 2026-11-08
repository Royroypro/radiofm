export namespace audio {
	
	export class Device {
	    id: string;
	    name: string;
	    kind: string;
	    is_default: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Device(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.is_default = source["is_default"];
	    }
	}

}

export namespace config {
	
	export class AudioSettings {
	    output_device: string;
	    preview_device: string;
	    mic_device: string;
	    mic_gain_db: number;
	    crossfade_ms: number;
	    duck_depth_db: number;
	    duck_threshold_db: number;
	    duck_attack_ms: number;
	    duck_release_ms: number;
	    hard_fade_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new AudioSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.output_device = source["output_device"];
	        this.preview_device = source["preview_device"];
	        this.mic_device = source["mic_device"];
	        this.mic_gain_db = source["mic_gain_db"];
	        this.crossfade_ms = source["crossfade_ms"];
	        this.duck_depth_db = source["duck_depth_db"];
	        this.duck_threshold_db = source["duck_threshold_db"];
	        this.duck_attack_ms = source["duck_attack_ms"];
	        this.duck_release_ms = source["duck_release_ms"];
	        this.hard_fade_ms = source["hard_fade_ms"];
	    }
	}
	export class IcecastSettings {
	    enabled: boolean;
	    host: string;
	    port: number;
	    mount: string;
	    user: string;
	    format: string;
	    bitrate_kbps: number;
	    source: string;
	    capture_device: string;
	    metadata_music_only: boolean;
	
	    static createFrom(source: any = {}) {
	        return new IcecastSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.mount = source["mount"];
	        this.user = source["user"];
	        this.format = source["format"];
	        this.bitrate_kbps = source["bitrate_kbps"];
	        this.source = source["source"];
	        this.capture_device = source["capture_device"];
	        this.metadata_music_only = source["metadata_music_only"];
	    }
	}
	export class InterfaceSettings {
	    cart_page: number;
	
	    static createFrom(source: any = {}) {
	        return new InterfaceSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cart_page = source["cart_page"];
	    }
	}
	export class LocutionSettings {
	    dir: string;
	    hour_prefix: string;
	    minute_prefix: string;
	    temp_prefix: string;
	
	    static createFrom(source: any = {}) {
	        return new LocutionSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dir = source["dir"];
	        this.hour_prefix = source["hour_prefix"];
	        this.minute_prefix = source["minute_prefix"];
	        this.temp_prefix = source["temp_prefix"];
	    }
	}
	export class RotationSettings {
	    artist_separation_min: number;
	    title_separation_hours: number;
	
	    static createFrom(source: any = {}) {
	        return new RotationSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.artist_separation_min = source["artist_separation_min"];
	        this.title_separation_hours = source["title_separation_hours"];
	    }
	}
	export class SafetySettings {
	    silence_seconds: number;
	    auto_start: boolean;
	    start_playing: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SafetySettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.silence_seconds = source["silence_seconds"];
	        this.auto_start = source["auto_start"];
	        this.start_playing = source["start_playing"];
	    }
	}
	export class ScheduleSettings {
	    days_ahead: number;
	    fill_tolerance_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new ScheduleSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.days_ahead = source["days_ahead"];
	        this.fill_tolerance_ms = source["fill_tolerance_ms"];
	    }
	}
	export class ServerSettings {
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new ServerSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	    }
	}
	export class StationSettings {
	    name: string;
	    city: string;
	    timezone: string;
	    latitude: number;
	    longitude: number;
	
	    static createFrom(source: any = {}) {
	        return new StationSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.city = source["city"];
	        this.timezone = source["timezone"];
	        this.latitude = source["latitude"];
	        this.longitude = source["longitude"];
	    }
	}
	export class Settings {
	    audio: AudioSettings;
	    icecast: IcecastSettings;
	    server: ServerSettings;
	    station: StationSettings;
	    rotation: RotationSettings;
	    locutions: LocutionSettings;
	    safety: SafetySettings;
	    schedule: ScheduleSettings;
	    ui: InterfaceSettings;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.audio = this.convertValues(source["audio"], AudioSettings);
	        this.icecast = this.convertValues(source["icecast"], IcecastSettings);
	        this.server = this.convertValues(source["server"], ServerSettings);
	        this.station = this.convertValues(source["station"], StationSettings);
	        this.rotation = this.convertValues(source["rotation"], RotationSettings);
	        this.locutions = this.convertValues(source["locutions"], LocutionSettings);
	        this.safety = this.convertValues(source["safety"], SafetySettings);
	        this.schedule = this.convertValues(source["schedule"], ScheduleSettings);
	        this.ui = this.convertValues(source["ui"], InterfaceSettings);
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

export namespace domain {
	
	export class Category {
	    id: number;
	    name: string;
	    parent_id: number;
	    kind: string;
	    color: string;
	
	    static createFrom(source: any = {}) {
	        return new Category(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.parent_id = source["parent_id"];
	        this.kind = source["kind"];
	        this.color = source["color"];
	    }
	}
	export class Track {
	    id: number;
	    path: string;
	    title: string;
	    artist: string;
	    album: string;
	    genre: string;
	    year: string;
	    category_id: number;
	    duration: number;
	    active: boolean;
	    // Go type: time
	    valid_from?: any;
	    // Go type: time
	    valid_to?: any;
	    analyzed: boolean;
	    loudness: number;
	    gain_db: number;
	    cue_in: number;
	    cue_out: number;
	    segue_at: number;
	    intro: number;
	    analyze_err: string;
	    missing: boolean;
	    // Go type: time
	    last_played_at?: any;
	    play_count: number;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Track(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.path = source["path"];
	        this.title = source["title"];
	        this.artist = source["artist"];
	        this.album = source["album"];
	        this.genre = source["genre"];
	        this.year = source["year"];
	        this.category_id = source["category_id"];
	        this.duration = source["duration"];
	        this.active = source["active"];
	        this.valid_from = this.convertValues(source["valid_from"], null);
	        this.valid_to = this.convertValues(source["valid_to"], null);
	        this.analyzed = source["analyzed"];
	        this.loudness = source["loudness"];
	        this.gain_db = source["gain_db"];
	        this.cue_in = source["cue_in"];
	        this.cue_out = source["cue_out"];
	        this.segue_at = source["segue_at"];
	        this.intro = source["intro"];
	        this.analyze_err = source["analyze_err"];
	        this.missing = source["missing"];
	        this.last_played_at = this.convertValues(source["last_played_at"], null);
	        this.play_count = source["play_count"];
	        this.created_at = this.convertValues(source["created_at"], null);
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
	export class TrackFilter {
	    CategoryID: number;
	    Query: string;
	    OnlyActive: boolean;
	    Limit: number;
	    Offset: number;
	
	    static createFrom(source: any = {}) {
	        return new TrackFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.CategoryID = source["CategoryID"];
	        this.Query = source["Query"];
	        this.OnlyActive = source["OnlyActive"];
	        this.Limit = source["Limit"];
	        this.Offset = source["Offset"];
	    }
	}

}

export namespace library {
	
	export class AnalyzerStatus {
	    current: string;
	    pending: number;
	
	    static createFrom(source: any = {}) {
	        return new AnalyzerStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.current = source["current"];
	        this.pending = source["pending"];
	    }
	}
	export class ImportResult {
	    added: number;
	    updated: number;
	    failed: number;
	    errors: string[];
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.added = source["added"];
	        this.updated = source["updated"];
	        this.failed = source["failed"];
	        this.errors = source["errors"];
	    }
	}

}

export namespace ui {
	
	export class AppInfo {
	    name: string;
	    version: string;
	    data_dir: string;
	    ffmpeg: string;
	    ffmpeg_ok: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.data_dir = source["data_dir"];
	        this.ffmpeg = source["ffmpeg"];
	        this.ffmpeg_ok = source["ffmpeg_ok"];
	    }
	}
	export class CategoryView {
	    id: number;
	    name: string;
	    parent_id: number;
	    kind: string;
	    color: string;
	    tracks: number;
	
	    static createFrom(source: any = {}) {
	        return new CategoryView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.parent_id = source["parent_id"];
	        this.kind = source["kind"];
	        this.color = source["color"];
	        this.tracks = source["tracks"];
	    }
	}
	export class TrackPage {
	    items: domain.Track[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new TrackPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], domain.Track);
	        this.total = source["total"];
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

