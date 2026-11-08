export namespace app {
	
	export class AudioState {
	    output: string;
	    mic: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new AudioState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.output = source["output"];
	        this.mic = source["mic"];
	        this.error = source["error"];
	    }
	}

}

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
	export class Levels {
	    PeakL: number;
	    PeakR: number;
	    RMSL: number;
	    RMSR: number;
	
	    static createFrom(source: any = {}) {
	        return new Levels(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.PeakL = source["PeakL"];
	        this.PeakR = source["PeakR"];
	        this.RMSL = source["RMSL"];
	        this.RMSR = source["RMSR"];
	    }
	}
	export class SlotState {
	    loaded: boolean;
	    playing: boolean;
	    tag: number;
	    position: number;
	    length: number;
	
	    static createFrom(source: any = {}) {
	        return new SlotState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.loaded = source["loaded"];
	        this.playing = source["playing"];
	        this.tag = source["tag"];
	        this.position = source["position"];
	        this.length = source["length"];
	    }
	}
	export class Status {
	    decks: SlotState[];
	    carts_on: number[];
	    levels: Levels;
	    duck_gain: number;
	    limiter_gain: number;
	    mic_on: boolean;
	    dropouts: number;
	    enc_dropped: number;
	    rendered: number;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.decks = this.convertValues(source["decks"], SlotState);
	        this.carts_on = source["carts_on"];
	        this.levels = this.convertValues(source["levels"], Levels);
	        this.duck_gain = source["duck_gain"];
	        this.limiter_gain = source["limiter_gain"];
	        this.mic_on = source["mic_on"];
	        this.dropouts = source["dropouts"];
	        this.enc_dropped = source["enc_dropped"];
	        this.rendered = source["rendered"];
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
	
	export class Cart {
	    page: number;
	    slot: number;
	    label: string;
	    color: string;
	    path: string;
	    stop_on_push: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Cart(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.page = source["page"];
	        this.slot = source["slot"];
	        this.label = source["label"];
	        this.color = source["color"];
	        this.path = source["path"];
	        this.stop_on_push = source["stop_on_push"];
	    }
	}
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
	export class ClockSlot {
	    kind: string;
	    category_id: number;
	    track_id: number;
	    hard: boolean;
	    minute: number;
	    mode: string;
	    seconds: number;
	
	    static createFrom(source: any = {}) {
	        return new ClockSlot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.category_id = source["category_id"];
	        this.track_id = source["track_id"];
	        this.hard = source["hard"];
	        this.minute = source["minute"];
	        this.mode = source["mode"];
	        this.seconds = source["seconds"];
	    }
	}
	export class Clock {
	    id: number;
	    name: string;
	    color: string;
	    slots: ClockSlot[];
	
	    static createFrom(source: any = {}) {
	        return new Clock(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.color = source["color"];
	        this.slots = this.convertValues(source["slots"], ClockSlot);
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
	
	export class GridCell {
	    weekday: number;
	    hour: number;
	    clock_id: number;
	
	    static createFrom(source: any = {}) {
	        return new GridCell(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.weekday = source["weekday"];
	        this.hour = source["hour"];
	        this.clock_id = source["clock_id"];
	    }
	}
	export class GridException {
	    date: string;
	    hour: number;
	    clock_id: number;
	
	    static createFrom(source: any = {}) {
	        return new GridException(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.date = source["date"];
	        this.hour = source["hour"];
	        this.clock_id = source["clock_id"];
	    }
	}
	export class LogItem {
	    id: number;
	    day: string;
	    hour: number;
	    seq: number;
	    kind: string;
	    track_id: number;
	    category_id: number;
	    hard: boolean;
	    // Go type: time
	    at: any;
	    length: number;
	    optional: boolean;
	    locked: boolean;
	    status: string;
	    title: string;
	    artist: string;
	    mode: string;
	    seconds: number;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new LogItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.day = source["day"];
	        this.hour = source["hour"];
	        this.seq = source["seq"];
	        this.kind = source["kind"];
	        this.track_id = source["track_id"];
	        this.category_id = source["category_id"];
	        this.hard = source["hard"];
	        this.at = this.convertValues(source["at"], null);
	        this.length = source["length"];
	        this.optional = source["optional"];
	        this.locked = source["locked"];
	        this.status = source["status"];
	        this.title = source["title"];
	        this.artist = source["artist"];
	        this.mode = source["mode"];
	        this.seconds = source["seconds"];
	        this.note = source["note"];
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
	export class Play {
	    id: number;
	    track_id: number;
	    spot_id: number;
	    kind: string;
	    artist: string;
	    title: string;
	    // Go type: time
	    started_at: any;
	    duration: number;
	    complete: boolean;
	    uuid: string;
	
	    static createFrom(source: any = {}) {
	        return new Play(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.track_id = source["track_id"];
	        this.spot_id = source["spot_id"];
	        this.kind = source["kind"];
	        this.artist = source["artist"];
	        this.title = source["title"];
	        this.started_at = this.convertValues(source["started_at"], null);
	        this.duration = source["duration"];
	        this.complete = source["complete"];
	        this.uuid = source["uuid"];
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
	export class Spot {
	    id: number;
	    remote_id: number;
	    advertiser: string;
	    name: string;
	    path: string;
	    sha256: string;
	    duration: number;
	    // Go type: time
	    starts_at?: any;
	    // Go type: time
	    ends_at?: any;
	    plays_per_day: number;
	    hour_from: number;
	    hour_to: number;
	    active: boolean;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Spot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.remote_id = source["remote_id"];
	        this.advertiser = source["advertiser"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.sha256 = source["sha256"];
	        this.duration = source["duration"];
	        this.starts_at = this.convertValues(source["starts_at"], null);
	        this.ends_at = this.convertValues(source["ends_at"], null);
	        this.plays_per_day = source["plays_per_day"];
	        this.hour_from = source["hour_from"];
	        this.hour_to = source["hour_to"];
	        this.active = source["active"];
	        this.updated_at = this.convertValues(source["updated_at"], null);
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

export namespace ffmpeg {
	
	export class EncoderStatus {
	    connected: boolean;
	    // Go type: time
	    since?: any;
	    retries: number;
	    last_error?: string;
	    kbps: number;
	    bytes_sent: number;
	    format: string;
	
	    static createFrom(source: any = {}) {
	        return new EncoderStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connected = source["connected"];
	        this.since = this.convertValues(source["since"], null);
	        this.retries = source["retries"];
	        this.last_error = source["last_error"];
	        this.kbps = source["kbps"];
	        this.bytes_sent = source["bytes_sent"];
	        this.format = source["format"];
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

export namespace onair {
	
	export class Segment {
	    item_id: number;
	    index: number;
	    last: boolean;
	    kind: string;
	    path: string;
	    track_id: number;
	    spot_id: number;
	    remote_id: number;
	    artist: string;
	    title: string;
	    gain_db: number;
	    cue_in: number;
	    length: number;
	    segue: number;
	    intro: number;
	    silence: number;
	    mode: string;
	    music: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Segment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.item_id = source["item_id"];
	        this.index = source["index"];
	        this.last = source["last"];
	        this.kind = source["kind"];
	        this.path = source["path"];
	        this.track_id = source["track_id"];
	        this.spot_id = source["spot_id"];
	        this.remote_id = source["remote_id"];
	        this.artist = source["artist"];
	        this.title = source["title"];
	        this.gain_db = source["gain_db"];
	        this.cue_in = source["cue_in"];
	        this.length = source["length"];
	        this.segue = source["segue"];
	        this.intro = source["intro"];
	        this.silence = source["silence"];
	        this.mode = source["mode"];
	        this.music = source["music"];
	    }
	}
	export class DeckView {
	    playing: boolean;
	    current: boolean;
	    segment?: Segment;
	    position: number;
	    remaining: number;
	    intro_left: number;
	
	    static createFrom(source: any = {}) {
	        return new DeckView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.playing = source["playing"];
	        this.current = source["current"];
	        this.segment = this.convertValues(source["segment"], Segment);
	        this.position = source["position"];
	        this.remaining = source["remaining"];
	        this.intro_left = source["intro_left"];
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
	
	export class Warning {
	    // Go type: time
	    at: any;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Warning(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.at = this.convertValues(source["at"], null);
	        this.message = source["message"];
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
	export class Snapshot {
	    running: boolean;
	    mode: string;
	    waiting: boolean;
	    decks: DeckView[];
	    item_id: number;
	    next: domain.LogItem[];
	    silence_left: number;
	    engine: audio.Status;
	    carts_on: number[];
	    warnings: Warning[];
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.mode = source["mode"];
	        this.waiting = source["waiting"];
	        this.decks = this.convertValues(source["decks"], DeckView);
	        this.item_id = source["item_id"];
	        this.next = this.convertValues(source["next"], domain.LogItem);
	        this.silence_left = source["silence_left"];
	        this.engine = this.convertValues(source["engine"], audio.Status);
	        this.carts_on = source["carts_on"];
	        this.warnings = this.convertValues(source["warnings"], Warning);
	        this.updated_at = this.convertValues(source["updated_at"], null);
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

export namespace reports {
	
	export class ProofRow {
	    // Go type: time
	    at: any;
	    advertiser: string;
	    spot: string;
	    duration: number;
	    complete: boolean;
	    uuid: string;
	
	    static createFrom(source: any = {}) {
	        return new ProofRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.at = this.convertValues(source["at"], null);
	        this.advertiser = source["advertiser"];
	        this.spot = source["spot"];
	        this.duration = source["duration"];
	        this.complete = source["complete"];
	        this.uuid = source["uuid"];
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

export namespace spots {
	
	export class TandaView {
	    log_item_id: number;
	    // Go type: time
	    at: any;
	    status: string;
	    spots: domain.Spot[];
	
	    static createFrom(source: any = {}) {
	        return new TandaView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.log_item_id = source["log_item_id"];
	        this.at = this.convertValues(source["at"], null);
	        this.status = source["status"];
	        this.spots = this.convertValues(source["spots"], domain.Spot);
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

export namespace streaming {
	
	export class Status {
	    enabled: boolean;
	    has_password: boolean;
	    source: string;
	    target: string;
	    encoder: ffmpeg.EncoderStatus;
	    error: string;
	    song: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.has_password = source["has_password"];
	        this.source = source["source"];
	        this.target = source["target"];
	        this.encoder = this.convertValues(source["encoder"], ffmpeg.EncoderStatus);
	        this.error = source["error"];
	        this.song = source["song"];
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

export namespace sync {
	
	export class Status {
	    configured: boolean;
	    online: boolean;
	    suspended: boolean;
	    unauthorized: boolean;
	    // Go type: time
	    last_ok: any;
	    last_error: string;
	    queue: number;
	    spots: number;
	    // Go type: time
	    last_spots: any;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.configured = source["configured"];
	        this.online = source["online"];
	        this.suspended = source["suspended"];
	        this.unauthorized = source["unauthorized"];
	        this.last_ok = this.convertValues(source["last_ok"], null);
	        this.last_error = source["last_error"];
	        this.queue = source["queue"];
	        this.spots = source["spots"];
	        this.last_spots = this.convertValues(source["last_spots"], null);
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
	export class OnAirView {
	    air: onair.Snapshot;
	    stream: streaming.Status;
	    audio: app.AudioState;
	    sync: sync.Status;
	
	    static createFrom(source: any = {}) {
	        return new OnAirView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.air = this.convertValues(source["air"], onair.Snapshot);
	        this.stream = this.convertValues(source["stream"], streaming.Status);
	        this.audio = this.convertValues(source["audio"], app.AudioState);
	        this.sync = this.convertValues(source["sync"], sync.Status);
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
	export class ReplanResult {
	    missing: Record<number, number>;
	
	    static createFrom(source: any = {}) {
	        return new ReplanResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.missing = source["missing"];
	    }
	}
	export class SpotView {
	    id: number;
	    remote_id: number;
	    advertiser: string;
	    name: string;
	    path: string;
	    sha256: string;
	    duration: number;
	    // Go type: time
	    starts_at?: any;
	    // Go type: time
	    ends_at?: any;
	    plays_per_day: number;
	    hour_from: number;
	    hour_to: number;
	    active: boolean;
	    // Go type: time
	    updated_at: any;
	    aired_today: number;
	    from_web: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SpotView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.remote_id = source["remote_id"];
	        this.advertiser = source["advertiser"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.sha256 = source["sha256"];
	        this.duration = source["duration"];
	        this.starts_at = this.convertValues(source["starts_at"], null);
	        this.ends_at = this.convertValues(source["ends_at"], null);
	        this.plays_per_day = source["plays_per_day"];
	        this.hour_from = source["hour_from"];
	        this.hour_to = source["hour_to"];
	        this.active = source["active"];
	        this.updated_at = this.convertValues(source["updated_at"], null);
	        this.aired_today = source["aired_today"];
	        this.from_web = source["from_web"];
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

