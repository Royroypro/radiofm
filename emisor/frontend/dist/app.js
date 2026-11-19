// Club25 Emisor — interfaz. Habla con Go por window.go.ui.App (bindings de Wails)
// y recibe el estado en vivo por los eventos "estado", "registro" y "dispositivos".
// Si se abre en un navegador normal (sin Wails) usa un backend de demostración,
// solo para revisar el diseño.
"use strict";

const $ = (s, el = document) => el.querySelector(s);
const $$ = (s, el = document) => [...el.querySelectorAll(s)];

// ---------- backend ----------
const api = window.go && window.go.ui && window.go.ui.App ? window.go.ui.App : demoBackend();
const on = (ev, fn) => (window.runtime ? window.runtime.EventsOn(ev, fn) : api._on(ev, fn));

let state = null;   // GetState()
let cfg = null;     // configuración que se está editando
let devices = [];
let lastStatus = { radio: { state: "detenido" }, tv: { state: "detenido" } };

const BITRATES = [16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320];
const STATE_LABEL = { detenido: "Detenido", conectando: "Conectando…", en_vivo: "● En vivo", reconectando: "Reconectando" };

// ---------- utilidades de la configuración ----------
function get(path) { return path.split(".").reduce((o, k) => (o == null ? o : o[k]), cfg); }
function set(path, v) {
  const keys = path.split(".");
  const last = keys.pop();
  keys.reduce((o, k) => o[k], cfg)[last] = v;
}

function opt(value, label, disabled = false) {
  const o = document.createElement("option");
  o.value = value; o.textContent = label; o.disabled = disabled;
  return o;
}

// ---------- selector de fuente ----------
// Cada fuente se elige en dos pasos: primero el TIPO y después la lista (o el campo) de ese tipo.
// Así los micrófonos, las salidas de la PC (altavoces) y las cámaras nunca se mezclan.
const KINDS = {
  audio: [
    ["dshow", "🎙 Micrófono o entrada de línea"],
    ["loopback", "🔊 Audio de la PC (altavoces / salidas de reproducción)"],
    ["test", "Tono de prueba (1 kHz)"],
  ],
  tvaudio: [
    ["radio", "📻 Audio de la radio (la misma entrada de la pestaña Radio)"],
    ["icecast", "📡 La radio desde el servidor (lo que oyen los oyentes)"],
    ["dshow", "🎙 Micrófono o entrada de línea"],
    ["loopback", "🔊 Audio de la PC (altavoces / salidas de reproducción)"],
    ["none", "Sin audio (silencio)"],
    ["test", "Tono de prueba (1 kHz)"],
  ],
  video: [
    ["dshow", "📷 Cámara o capturadora"],
    ["screen", "🖥 Captura de pantalla (todo el escritorio)"],
    ["url", "🌐 Video desde una dirección http/https"],
    ["rtsp", "📹 Cámara IP o servidor RTSP (rtsp://)"],
    ["test", "Barras de color de prueba"],
  ],
};
const ROLE_TITLE = { audio: "Entrada de audio", tvaudio: "Audio de la TV", video: "Video" };
const NOTES = {
  loopback: "Se envía todo lo que suena en esa salida (reproductor, navegador, programa de radio…). Si en esa misma salida se escucha la radio en vivo, habrá eco.",
  url: "Un archivo (MP4…) o un directo (HLS .m3u8, MPEG-TS, FLV) por http o https. Si el origen se corta, se reconecta solo.",
  rtsp: "Cámara IP, DVR/NVR o servidor RTSP. Si pide usuario y clave, van en la dirección: rtsp://usuario:clave@192.168.1.64:554/ruta (la ruta depende de la marca). Va por TCP y se reconecta solo si se corta.",
  radio: "Sale lo mismo que entra a la radio, sin retraso, aunque la radio no esté emitiendo. Si cambia la entrada de la radio, la TV cambia con ella, sin cortar.",
  icecast: "Se escucha la radio en el servidor, como un oyente: sale exactamente lo que está al aire, venga de esta PC o de otra (Club25 Estudio, Jazler…). Llega con unos segundos de retraso. Si la radio no está al aire, la TV va en silencio hasta que vuelva.",
};
// Campo de dirección para los tipos de video que la llevan.
const URL_KINDS = {
  url: { label: "Dirección del video", placeholder: "https://servidor/canal/index.m3u8" },
  rtsp: { label: "Dirección RTSP de la cámara", placeholder: "rtsp://usuario:clave@192.168.1.64:554/stream1" },
};
const NET_VIDEO = (k) => k in URL_KINDS; // el video viene de la red y puede traer audio propio
const lastURL = {}; // dirección escrita por tipo, para no perderla al cambiar de tipo

// hidePass oculta la clave de una dirección (rtsp://usuario:****@…) para mostrarla en pantalla.
function hidePass(u) { return String(u || "").replace(/^([a-z]+:\/\/[^:/@]+:)[^@/]*@/i, "$1****@"); }

// tvAudioTitle es el título del audio de la TV según el modo (con video por dirección o RTSP).
function tvAudioTitle() {
  if (!NET_VIDEO(cfg.tv.video.kind)) return "Audio de la TV";
  switch (cfg.tv.audio_mode) {
    case "video": return "Audio si el video no trae audio";
    case "both": return "Audio para mezclar con el del video";
    default: return "Audio de la TV (el del video no sale)";
  }
}

// renderAudioMode muestra el modo de audio solo con video por dirección o RTSP.
function renderAudioMode() {
  const box = $("#tv-audiomode");
  if (!box) return;
  box.classList.toggle("hidden", !NET_VIDEO(cfg.tv.video.kind));
  $("#tv-mix").classList.toggle("hidden", cfg.tv.audio_mode !== "both");
  const notes = {
    video: "Sale el audio que trae el video. Si el video no trae audio (por ejemplo, un video de fondo), sale el audio elegido abajo.",
    other: "Sale solo el audio elegido abajo (por ejemplo, la radio); el audio del video no se escucha.",
    both: "Salen los dos a la vez. Si el video no trae audio, sale solo el audio elegido abajo. Ajuste el volumen de cada uno.",
  };
  $("#tv-audiomode-note").textContent = notes[cfg.tv.audio_mode] || "";
}

// devicesFor devuelve las opciones de la lista para un tipo (o null si ese tipo no lleva lista).
function devicesFor(role, kind) {
  if (kind === "dshow") {
    const want = role === "video" ? "video" : "audio";
    return devices.filter((d) => d.kind === want).map((d) => [d.name, d.name]);
  }
  if (kind === "loopback") {
    return [["", "Salida predeterminada de Windows"]].concat(
      devices.filter((d) => d.kind === "playback").map((d) => [d.name, d.name]));
  }
  return null;
}

function renderSource(box) {
  const path = box.dataset.src, role = box.dataset.role;
  const cur = get(path);
  const uk = URL_KINDS[cur.kind];
  if (uk) lastURL[path + ":" + cur.kind] = cur.name;
  const title = role === "tvaudio" ? tvAudioTitle() : ROLE_TITLE[role];
  box.innerHTML = `
    <label class="kind">${title}<select class="src-kind"></select></label>
    <label class="dev list"><span class="dev-title"></span><select class="src-dev"></select></label>
    <label class="dev url"><span class="url-title"></span>
      <input class="src-url" spellcheck="false"></label>
    <p class="hint note"></p>`;
  const kindSel = $(".src-kind", box), devSel = $(".src-dev", box), url = $(".src-url", box);
  KINDS[role].forEach(([k, label]) => kindSel.append(opt(k, label)));
  kindSel.value = cur.kind;

  const list = devicesFor(role, cur.kind);
  $(".list", box).classList.toggle("hidden", !list);
  $(".url", box).classList.toggle("hidden", !uk);
  if (uk) {
    $(".url-title", box).textContent = uk.label;
    url.placeholder = uk.placeholder;
  }
  if (list) {
    $(".dev-title", box).textContent = cur.kind === "loopback" ? "Salida de reproducción (altavoces)"
      : role === "video" ? "Cámara o capturadora" : "Micrófono o entrada de línea";
    if (!list.length) devSel.append(opt("", devices.length ? "(no hay dispositivos de este tipo)" : "(buscando…)", true));
    list.forEach(([v, label]) => devSel.append(opt(v, label)));
    // Un dispositivo guardado que ahora no está: se muestra para no perderlo.
    if (cur.name && !list.some(([v]) => v === cur.name)) devSel.append(opt(cur.name, "⚠ " + cur.name + " (no conectado)"));
    if (cur.kind === "dshow" && !cur.name) devSel.prepend(opt("", "— elija —"));
    devSel.value = cur.name;
  }
  if (uk) url.value = cur.name;
  $(".note", box).textContent = NOTES[cur.kind] || "";

  kindSel.addEventListener("change", () => {
    const k = kindSel.value;
    let name = "";
    if (URL_KINDS[k]) name = lastURL[path + ":" + k] || "";
    const l = devicesFor(role, k);
    if (k === "dshow" && l && l.length) name = l[0][0];
    set(path, { kind: k, name });
    changeInput(path);
    renderAllSources();
    $(".src-kind", box).focus(); // el cuadro se redibujó: se devuelve el foco (teclado)
    scheduleSave();
  });
  devSel.addEventListener("change", () => { set(path, { kind: cur.kind, name: devSel.value }); changeInput(path); scheduleSave(); });
  url.addEventListener("change", () => {
    lastURL[path + ":" + cur.kind] = url.value.trim();
    set(path, { kind: cur.kind, name: url.value.trim() });
    markPending(path);
    scheduleSave();
  });
}

function renderAllSources() { $$(".src").forEach(renderSource); renderAudioMode(); }

function fillCodecs() {
  const sel = $("#codec");
  sel.innerHTML = "";
  state.codecs.forEach((c) => sel.append(opt(c.id, c.label + (c.note ? "  ·  " + c.note : ""), c.disabled)));
}

// Rangos por códec (iguales a config.AudioBitrateRange en Go).
function bitrateRange(codec, ch) {
  switch (codec) {
    case "heaac": return ch === 1 ? [16, 48] : [24, 64];
    case "heaacv2": return [16, 48];
    case "opus": return [16, 320];
    case "vorbis": return [48, 320];
    default: return [32, 320];
  }
}

function fillBitrates() {
  const [lo, hi] = bitrateRange(cfg.radio.codec, cfg.radio.channels);
  const sel = $("#bitrate");
  sel.innerHTML = "";
  BITRATES.filter((b) => b >= lo && b <= hi).forEach((b) => sel.append(opt(b, b + " kbps")));
  if (cfg.radio.bitrate_kbps < lo || cfg.radio.bitrate_kbps > hi) {
    // Ajusta al valor permitido más cercano.
    cfg.radio.bitrate_kbps = Math.min(Math.max(cfg.radio.bitrate_kbps, lo), hi);
  }
  sel.value = cfg.radio.bitrate_kbps;
  const hints = {
    mp3: "MP3: el más compatible (radios por internet, autos, apps). 128 kbps es lo habitual; hasta 320 kbps.",
    aac: "AAC-LC: mejor que MP3 al mismo bitrate. 96–128 kbps suena como MP3 de 160–192.",
    heaac: "AAC+ (HE-AAC): pensado para bitrates bajos (32–64 kbps), ideal para datos móviles.",
    heaacv2: "AAC+ v2: el de menor consumo (24–48 kbps), solo estéreo.",
    opus: "Opus: el más eficiente; 64–96 kbps suena muy bien. Siempre a 48 kHz.",
    vorbis: "Ogg Vorbis: alternativa libre a MP3.",
  };
  $("#codec-hint").textContent = hints[cfg.radio.codec] || "";
  const ch = $('[data-k="radio.sample_rate"]');
  ch.disabled = cfg.radio.codec === "opus";
}

function fillTVAudioBitrates() {
  const sel = $("#tv-akbps");
  sel.innerHTML = "";
  BITRATES.filter((b) => b >= 32 && b <= state.max_audio_kbps).forEach((b) => sel.append(opt(b, b + " kbps")));
  sel.value = cfg.tv.audio_kbps;
}

function fillEncoders() {
  const sel = $("#encoder");
  sel.innerHTML = "";
  state.encoders.forEach((e) => sel.append(opt(e.id, e.label + (e.note ? " (" + e.note + ")" : ""), e.disabled)));
  sel.value = cfg.tv.encoder;
}

// Copia cfg a los campos.
function render() {
  $$("[data-k]").forEach((el) => {
    const v = get(el.dataset.k);
    if (el.type === "checkbox") el.checked = !!v;
    else el.value = v ?? "";
  });
  renderAllSources();
  $$(".eqbox").forEach(renderEQ);
  $$("[data-gain]").forEach((r) => { r.value = cfg[r.dataset.gain].gain_db || 0; });
  if ($("#titles-hint")) renderTitles();
  fillBitrates();
  $('[data-k="radio.bitrate_kbps"]').value = cfg.radio.bitrate_kbps;
  fillTVAudioBitrates();
  fillEncoders();
  $("#resolution").value = cfg.tv.width + "x" + cfg.tv.height;
  const vr = $("#vkbps-range");
  vr.max = state.max_video_kbps;
  vr.value = cfg.tv.video_kbps;
  $("#vkbps").max = state.max_video_kbps;
  $("#max-video").textContent = state.max_video_kbps;
  updateCards();
}

// ---------- guardado automático ----------
let saveTimer = null;
function scheduleSave() {
  clearTimeout(saveTimer);
  saveTimer = setTimeout(() => api.SaveConfig(cfg).catch(showError), 400);
}

function bindInputs() {
  $$("[data-k]").forEach((el) => {
    el.addEventListener("change", () => {
      let v;
      if (el.type === "checkbox") v = el.checked;
      else if ("num" in el.dataset) v = Number(el.value);
      else v = el.value;
      set(el.dataset.k, v);
      markPending(el.dataset.k);
      el.classList.remove("invalid");
      if (el.dataset.k === "radio.codec" || el.dataset.k === "radio.channels") fillBitrates();
      if (el.dataset.k === "tv.video_kbps") $("#vkbps-range").value = v;
      if (el.dataset.k === "tv.audio_mode") renderAllSources(); // cambia el título del audio y la mezcla
      updateCards();
      scheduleSave();
    });
  });
  $("#vkbps-range").addEventListener("input", (e) => {
    cfg.tv.video_kbps = Number(e.target.value);
    markPending("tv.video_kbps");
    $("#vkbps").value = cfg.tv.video_kbps;
    scheduleSave();
  });
  $("#resolution").addEventListener("change", (e) => {
    const [w, h] = e.target.value.split("x").map(Number);
    cfg.tv.width = w; cfg.tv.height = h;
    markPending("tv.width");
    scheduleSave();
  });
  $$(".eye").forEach((b) => b.addEventListener("click", () => {
    const i = b.previousElementSibling;
    i.type = i.type === "password" ? "text" : "password";
  }));
  $$("[data-refresh]").forEach((b) => b.addEventListener("click", () => { devices = []; render(); api.RefreshDevices(); }));
  $$(".tab").forEach((t) => t.addEventListener("click", () => showTab(t.dataset.tab)));

  $$("[data-start]").forEach((b) => b.addEventListener("click", () => start(b.dataset.start)));
  $$("[data-stop]").forEach((b) => b.addEventListener("click", () => api.Stop(b.dataset.stop)));
  $("#start-all").addEventListener("click", () => start("todo"));
  $$("[data-apply]").forEach((b) => b.addEventListener("click", () => apply(b.dataset.apply)));
  $$("[data-enhance]").forEach((b) => b.addEventListener("click", () => toggleEnhance(b.dataset.enhance)));
  $$("[data-gain]").forEach((r) => {
    const which = r.dataset.gain;
    const num = $(`[data-k="${which}.gain_db"]`);
    r.addEventListener("input", () => { num.value = r.value; setGain(which, Number(r.value)); });
    r.addEventListener("dblclick", () => { r.value = 0; num.value = 0; setGain(which, 0); });
    num.addEventListener("change", () => { r.value = num.value; setGain(which, Number(num.value)); });
  });
  $("#stop-all").addEventListener("click", () => api.Stop("todo"));

  $("#import").addEventListener("click", () => {
    $("#import").disabled = true;
    api.ImportFromPanel(cfg).then((r) => {
      cfg = r.config; render(); markPending("radio.*"); markPending("tv.*");
      showOk("Datos traídos de " + (r.result.station || "la web") + ":\n• " + r.result.notes.join("\n• "));
    }).catch(showError).finally(() => ($("#import").disabled = false));
  });
  $("#apply-paste").addEventListener("click", () =>
    api.ParsePasted(cfg, $("#paste").value).then((r) => {
      cfg = r.config; render(); $("#paste").value = ""; markPending("radio.*"); markPending("tv.*");
      showOk("Datos cargados:\n• " + r.notes.join("\n• "));
    }).catch(showError));
  $("#reset").addEventListener("click", () =>
    api.ResetDefaults().then((c) => { cfg = c; render(); scheduleSave(); markPending("radio.*"); markPending("tv.*"); showOk("Valores por defecto restablecidos."); }));
}

function showTab(name) {
  $$(".tab").forEach((t) => t.classList.toggle("active", t.dataset.tab === name));
  $$(".panel").forEach((p) => p.classList.toggle("active", p.id === "tab-" + name));
}

function start(which) {
  clearTimeout(saveTimer);
  hideAlert();
  for (const n of ["radio", "tv"]) if (which === n || (which === "todo" && cfg[n].enabled)) pending[n] = false;
  updateCards();
  api.Start(which, cfg).catch((e) => {
    showError(e);
    // Lleva a la pestaña del primer error para que se vea qué falta.
    const msg = String(e);
    const where = msg.startsWith("TV") ? "tv" : msg.startsWith("Radio") ? "radio" : "";
    if (where) showSub(where, subForError(where, msg));
  });
}

// ---------- avisos ----------
function showError(e) {
  const a = $("#alert");
  a.textContent = String(e && e.message ? e.message : e);
  a.className = "alert";
}
function showOk(msg) {
  const a = $("#alert");
  a.textContent = msg;
  a.className = "alert ok";
  clearTimeout(showOk.t);
  showOk.t = setTimeout(hideAlert, 8000);
}
function hideAlert() { $("#alert").className = "alert hidden"; }

// ---------- tarjetas de estado y medidores ----------
function makeMeters(box, n) {
  if (box.childElementCount === n + 1) return;
  box.innerHTML = "";
  const names = n === 1 ? ["M"] : ["I", "D"];
  for (let i = 0; i < n; i++) {
    const m = document.createElement("div");
    m.className = "meter";
    m.innerHTML = `<span>${names[i] || i + 1}</span><div class="bar"><div class="fill"></div><div class="peak"></div></div><span class="db">-∞</span>`;
    box.append(m);
  }
  const sc = document.createElement("div");
  sc.className = "scale";
  sc.innerHTML = "<span>-60</span><span>-40</span><span>-20</span><span>-12</span><span>-6</span><span>0 dB</span>";
  box.append(sc);
}

// -60 dB → 0 %, 0 dB → 100 % (lineal en dB, como BUTT).
const pct = (db) => Math.max(0, Math.min(100, ((db + 60) / 60) * 100));

function drawMeters(name, st) {
  const box = $(`[data-meter="${name}"]`);
  const n = name === "radio" ? cfg.radio.channels : 2;
  makeMeters(box, n);
  const levels = st.state === "detenido" ? [] : st.levels || [];
  $$(".meter", box).forEach((m, i) => {
    const l = levels[i] || levels[0] || { peak: -90, rms: -90 };
    const w = pct(l.rms);
    const fill = $(".fill", m);
    fill.style.width = w + "%";
    fill.style.setProperty("--full", w > 0 ? (10000 / w) + "%" : "100%");
    $(".peak", m).style.left = pct(l.peak) + "%";
    $(".db", m).textContent = l.peak <= -90 ? "-∞" : l.peak.toFixed(1);
  });
}

function fmtTime(sec) {
  const h = Math.floor(sec / 3600), m = Math.floor((sec % 3600) / 60), s = Math.floor(sec % 60);
  return [h, m, s].map((x) => String(x).padStart(2, "0")).join(":");
}

function target(name) {
  if (name === "radio") return `${cfg.radio.host}:${cfg.radio.port}${cfg.radio.mount}`;
  const port = cfg.tv.protocol === "srt" ? cfg.tv.srt_port : cfg.tv.rtmp_port;
  return `${cfg.tv.protocol}://${cfg.tv.host}:${port}/${cfg.tv.path}`;
}

function updateCards() {
  if (!cfg) return;
  for (const name of ["radio", "tv"]) {
    const st = lastStatus[name] || { state: "detenido" };
    const card = $("#out-" + name);
    const running = st.state !== "detenido";
    card.classList.toggle("live", st.state === "en_vivo");
    card.classList.toggle("off", !cfg[name].enabled && !running);
    const pill = $(".pill", card);
    pill.dataset.state = st.state;
    pill.textContent = st.state === "reconectando" && st.retry_in_sec
      ? `Reconectando en ${st.retry_in_sec} s (intento ${st.attempt})` : STATE_LABEL[st.state] || st.state;
    const live = st.state === "en_vivo" && st.live_since && !st.live_since.startsWith("0001");
    $('[data-info="time"]', card).textContent = live ? fmtTime((Date.now() - Date.parse(st.live_since)) / 1000) : "00:00:00";
    let rate = "—";
    if (live && st.bitrate_kbps > 0) rate = Math.round(st.bitrate_kbps) + " kbps";
    if (live && name === "tv" && st.fps) rate += ` · ${st.fps.toFixed(0)} fps`;
    if (live && st.speed && st.speed < 0.95) rate += ` · ⚠ lento ×${st.speed.toFixed(2)}`;
    $('[data-info="rate"]', card).textContent = rate;
    $('[data-info="target"]', card).textContent = target(name);
    const msg = $('[data-info="msg"]', card);
    msg.textContent = st.message || "";
    msg.classList.toggle("bad", st.state === "reconectando" || (st.message || "").startsWith("Detenido:"));
    $(`[data-start="${name}"]`).disabled = running;
    $$(`[data-enhance="${name}"]`).forEach((enh) => {
      enh.classList.toggle("on", !!cfg[name].enhance);
      enh.textContent = cfg[name].enhance ? "✦ MEJORA activa" : "✦ MEJORA";
    });
    const pend = running && pending[name];
    $$(`[data-apply="${name}"]`).forEach((b) => b.classList.toggle("pending", pend));
    $(`[data-apply="${name}"]`, card).disabled = !pend;
    const bar = $(`[data-applybar="${name}"]`);
    bar.classList.toggle("pending", pend);
    bar.classList.toggle("live", running && !pend);
    const label = name === "radio" ? "La radio" : "La TV";
    $(".ab-msg", bar).textContent = pend
      ? `Hay cambios sin aplicar: ${label.toLowerCase()} sigue al aire con la configuración anterior. Pulse Aplicar para reconectar con los cambios.`
      : running ? `${label} está al aire con esta configuración.`
        : "Los cambios se guardan solos y se usan al iniciar.";
    $(`[data-stop="${name}"]`).disabled = !running;
    drawMeters(name, st);
  }
  renderSummary();
  const any = ["radio", "tv"].some((n) => (lastStatus[n] || {}).state !== "detenido");
  $("#stop-all").disabled = !any;
}

// ---------- registro ----------
function addLog(line) {
  const pre = $("#log");
  const atEnd = pre.scrollTop + pre.clientHeight >= pre.scrollHeight - 20;
  pre.textContent += line + "\n";
  if (pre.textContent.length > 200000) pre.textContent = pre.textContent.slice(-150000);
  if (atEnd) pre.scrollTop = pre.scrollHeight;
}

// ---------- arranque ----------
async function init() {
  state = await api.GetState();
  cfg = state.config;
  devices = state.devices || [];
  $("#version").textContent = "v" + state.version;
  $("#cfg-path").textContent = state.config_path || "—";
  $("#ff-path").textContent = state.ffmpeg || "no encontrado";
  const ff = $("#ffmpeg-status");
  if (state.ffmpeg_error) { ff.textContent = "⚠ " + state.ffmpeg_error; showError(state.ffmpeg_error); }
  eqProfiles = state.eq_profiles || [];
  if (state.eq_bands) eqBands = state.eq_bands;
  fillCodecs();
  render();
  license = state.license || license;
  update = state.update || update;
  bindInputs();
  bindSubtabs();
  bindLicense();
  renderLicense();
  bindTitles();
  renderTitles();
  showNowPlaying(state.now_playing);
  api.LastPlaylist().then((pl) => { if (pl && pl.titles && pl.titles.length) { playlist = pl; renderPlaylist(); } else renderPlaylist(); });
  on("sonando", showNowPlaying);
  (state.logs || []).forEach(addLog);
  on("estado", (s) => { lastStatus = s; updateCards(); });
  on("registro", addLog);
  on("dispositivos", (list) => {
    devices = list || [];
    renderAllSources();
    if (!devices.length) addLog("No se encontraron micrófonos ni cámaras (¿ffmpeg sin dshow?).");
  });
  setInterval(updateCards, 1000); // el reloj de «en vivo» avanza aunque no lleguen eventos
}

init().catch(showError);

// ---------- backend de demostración (solo en un navegador, sin Wails) ----------
function demoBackend() {
  const handlers = {};
  const emit = (ev, d) => (handlers[ev] || []).forEach((f) => f(d));
  const st = { radio: { state: "detenido" }, tv: { state: "detenido" } };
  let c = null;
  const tick = () => {
    for (const n of ["radio", "tv"]) {
      if (st[n].state === "en_vivo") {
        const p = -30 + Math.random() * 26;
        st[n].levels = [{ peak: p, rms: p - 8 }, { peak: p - 2, rms: p - 10 }];
        st[n].bitrate_kbps = n === "radio" ? c.radio.bitrate_kbps : c.tv.video_kbps + c.tv.audio_kbps;
        st[n].fps = c.tv.fps; st[n].speed = 1;
      }
    }
    emit("estado", JSON.parse(JSON.stringify(st)));
  };
  setInterval(tick, 100);
  return {
    _on: (ev, fn) => (handlers[ev] = handlers[ev] || []).push(fn),
    GetState: async () => {
      c = {
        radio: { enabled: true, host: "radioclub25peru.com", port: 8010, mount: "/radio", user: "source", password: "",
          codec: "mp3", bitrate_kbps: 128, sample_rate: 44100, channels: 2, input: { kind: "dshow", name: "Micrófono (USB)" },
          gain_db: 0, eq: { enabled: true, preset: "Radio FM", gains: [3, 2.5, 1.5, 0, -0.5, 0, 1, 2, 3, 3.5] },
          titles: { source: "jazler", file: "C:\\Jazler RadioStar 2\\Exports\\NowOnAir.xml", idle_text: "Radio Club 25", playlist: "" },
          stream_name: "Radio Club 25", genre: "Variado", description: "En vivo desde Huacho", website: "https://radioclub25peru.com" },
        tv: { enabled: false, protocol: "rtmp", host: "radioclub25peru.com", rtmp_port: 1935, srt_port: 8890, path: "tv",
          user: "emisora", password: "", video: { kind: "screen", name: "" }, audio: { kind: "radio", name: "" }, audio_mode: "video", video_mix_db: 0, other_mix_db: -6,
          width: 1280, height: 720, fps: 30, video_kbps: 2500, audio_kbps: 128, encoder: "libx264", keyframe_sec: 2, gain_db: 0, eq: { enabled: false, preset: "Plano", gains: [0, 0, 0, 0, 0, 0, 0, 0, 0, 0] } },
        panel: { url: "https://radioclub25peru.com", token: "" }, ffmpeg_path: "",
        startup: { with_windows: false, auto_broadcast: false, background: true }, eq_profiles: [],
      };
      return {
        version: "demo", config: c, config_path: "(demostración)", ffmpeg: "(demostración)", ffmpeg_error: "",
        devices: [{ name: "Micrófono (USB)", kind: "audio" }, { name: "Line 1/2 (M-Track)", kind: "audio" }, { name: "Cámara USB", kind: "video" }, { name: "Altavoces (Realtek(R) Audio)", kind: "playback" }, { name: "CABLE Input (VB-Audio Virtual Cable)", kind: "playback" }],
        codecs: ["mp3", "aac", "heaac", "heaacv2", "opus", "vorbis"].map((id) => ({ id, label: id.toUpperCase(), disabled: id.startsWith("heaac"), note: "" })),
        encoders: [{ id: "libx264", label: "x264 (procesador)" }, { id: "h264_nvenc", label: "NVIDIA NVENC" }],
        logs: ["Modo demostración: abra el programa para emitir."], max_audio_kbps: 320, max_video_kbps: 6000,
        eq_profiles: [["Plano", [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]], ["Radio FM", [3, 2.5, 1.5, 0, -0.5, 0, 1, 2, 3, 3.5]],
          ["Modo fiesta", [6, 5, 3, 0, -1.5, -1, 0, 2, 4, 5]], ["Voz / programa hablado", [-6, -4, -1, 0, 1, 2, 3, 3, 1, 0]]]
          .map(([name, gains]) => ({ name, gains, built_in: true })),
        now_playing: { title: "Grupo 5 - Motor y motivo", source: "jazler", kind: "song", sent: true },
        license: { authenticated: location.hash !== "#bloqueado", reason: "El token no es válido para esa web. Genere uno en Admin → Estudio (PC).",
          base: "https://radioclub25peru.com", station: { name: "Radio Club 25", frequency: "93.3 FM" }, checked_at: new Date().toISOString(),
          provider: { phone: "9488933154", url: "https://sehuacho.com/radio" } },
        update: location.hash === "#actualizar" ? { available: true, current: "1.5.0", version: "1.6.0", notes: "Menú ordenado y mejoras de estabilidad." } : { available: false },
      };
    },
    SaveConfig: async (x) => { c = x; },
    RefreshDevices: () => {},
    Start: async (w, x) => {
      c = x;
      for (const n of ["radio", "tv"]) if (w === n || (w === "todo" && c[n].enabled)) st[n] = { state: "en_vivo", live_since: new Date().toISOString() };
    },
    Stop: async (w) => { for (const n of ["radio", "tv"]) if (w === n || w === "todo") st[n] = { state: "detenido" }; },
    SendSong: async () => {}, SetEQ: async () => {}, SetGain: async () => {},
    Authenticate: async (u) => { if (!u) throw "Escriba el dominio de la web y el token del estudio."; return { authenticated: true, base: "https://" + u, station: { name: "Radio Club 25", frequency: "93.3 FM" }, provider: { phone: "9488933154", url: "https://sehuacho.com/radio" } }; },
    CheckLicense: async () => ({ authenticated: false, reason: "Demostración", provider: { phone: "9488933154", url: "https://sehuacho.com/radio" } }),
    OpenURL: async () => {}, RestoreConfig: async () => { throw "Demostración."; }, ApplyUpdate: async () => { throw "Demostración."; }, SaveEQProfile: async () => [], DeleteEQProfile: async () => [],
    LoadPlaylist: async () => ({ path: "D:\\Listas\\Sabado.m3u", titles: ["Grupo 5 - Motor y motivo", "Agua Marina - Paloma ajena", "Corazón Serrano - Mía"] }),
    LastPlaylist: async () => ({ path: "", titles: [] }), DetectJazler: async () => "C:\\Jazler RadioStar 2\\Exports\\NowOnAir.xml", Apply: async (w) => st[w].state !== "detenido", SetInput: async () => false, SetEnhance: async () => {}, ImportFromPanel: async () => { throw "Demostración: sin conexión."; },
    ParsePasted: async () => { throw "Demostración."; }, ResetDefaults: async () => c,
  };
}

// ---------- Aplicar y MEJORA ----------
// pending: hay cambios hechos con la salida al aire que todavía no se aplicaron.
const pending = { radio: false, tv: false };

// markPending marca la sección de un campo ("radio.codec" → radio). «Emitir» no requiere reiniciar.
function markPending(path) {
  const sec = path.split(".")[0];
  if (!(sec in pending) || path.endsWith(".enabled") || path.includes(".titles.") || path.includes(".eq") || path.endsWith(".gain_db")) return;
  const st = (lastStatus[sec] || {}).state;
  if (st && st !== "detenido") pending[sec] = true;
  updateCards();
}

// Las entradas de audio se cambian al aire, sin reconectar: no esperan a «Aplicar».
const LIVE_INPUTS = { "radio.input": "radio", "tv.audio": "tv" };

function changeInput(path) {
  const which = LIVE_INPUTS[path];
  if (!which) { markPending(path); return; }
  const src = which === "radio" ? cfg.radio.input : cfg.tv.audio;
  if (src.kind === "dshow" && !src.name) return; // falta elegir el dispositivo
  const st = (lastStatus[which] || {}).state;
  if (!st || st === "detenido") return; // se guarda y se usa al iniciar
  clearTimeout(saveTimer);
  const label = which === "radio" ? "La radio" : "La TV";
  api.SetInput(which, src).then((restarted) => {
    showOk(restarted ? `${label} se está reconectando con la entrada nueva…` : `${label}: entrada cambiada al aire, sin cortar.`);
  }).catch(showError);
}

function apply(which) {
  clearTimeout(saveTimer);
  hideAlert();
  api.Apply(which, cfg).then((restarted) => {
    pending[which] = false;
    updateCards();
    const label = which === "radio" ? "La radio" : "La TV";
    const live = (lastStatus[which] || {}).state;
    showOk(restarted ? `${label} se está reconectando con los cambios…`
      : live && live !== "detenido" ? `${label}: cambios aplicados al aire, sin cortar.` : "Guardado: se usará al iniciar.");
  }).catch((e) => { showError(e); showSub(which, subForError(which, String(e))); });
}

function toggleEnhance(which) {
  const on = !cfg[which].enhance;
  cfg[which].enhance = on;
  updateCards();
  api.SetEnhance(which, on).catch((e) => { cfg[which].enhance = !on; updateCards(); showError(e); });
}

// ---------- ecualizador ----------
// 10 bandas que se cambian al aire, sin cortar. Perfiles de fábrica y propios.
let eqProfiles = [];
let eqBands = [32, 64, 125, 250, 500, 1000, 2000, 4000, 8000, 16000];
const eqTimers = {};

function bandLabel(f) { return f >= 1000 ? f / 1000 + "k" : String(f); }

function sendEQ(which, delay = 150) {
  clearTimeout(eqTimers[which]);
  eqTimers[which] = setTimeout(() => api.SetEQ(which, cfg[which].eq).catch(showError), delay);
}

function renderEQ(box) {
  const which = box.dataset.eq;
  const eq = cfg[which].eq;
  box.innerHTML = `
    <h3 class="sub">Ecualizador</h3>
    <div class="eq-top">
      <label class="check"><input type="checkbox" class="eq-on"> Activar ecualizador</label>
      <label class="eq-sel">Perfil <select class="eq-preset"></select></label>
      <button class="btn ghost eq-flat" type="button">Poner en plano</button>
    </div>
    <div class="eq-bands"></div>
    <div class="eq-save-row">
      <input class="eq-name" placeholder="Nombre para guardar este ajuste (ej.: Mañanas, Fiesta del sábado)">
      <button class="btn eq-save" type="button">💾 Guardar perfil</button>
      <button class="btn ghost eq-del" type="button">Borrar perfil</button>
    </div>
    <p class="hint">Al elegir un perfil, el cambio completo tarda unos 2 segundos. Un limitador final evita que el audio
      sature aunque suba bandas. Doble clic en una banda (o en la ganancia) la deja en 0.</p>`;
  const on = $(".eq-on", box), sel = $(".eq-preset", box), bands = $(".eq-bands", box);
  on.checked = eq.enabled;
  box.classList.toggle("eq-off", !eq.enabled);

  // Perfiles: de fábrica y propios, más «ajuste manual».
  const own = eqProfiles.filter((p) => !p.built_in), builtin = eqProfiles.filter((p) => p.built_in);
  sel.append(opt("", "— Ajuste manual —"));
  const g1 = document.createElement("optgroup"); g1.label = "De fábrica";
  builtin.forEach((p) => g1.append(opt(p.name, p.name)));
  sel.append(g1);
  if (own.length) {
    const g2 = document.createElement("optgroup"); g2.label = "Mis perfiles";
    own.forEach((p) => g2.append(opt(p.name, p.name)));
    sel.append(g2);
  }
  sel.value = eqProfiles.some((p) => p.name === eq.preset) ? eq.preset : "";
  $(".eq-del", box).disabled = !own.some((p) => p.name === eq.preset);

  eqBands.forEach((f, i) => {
    const b = document.createElement("div");
    b.className = "band";
    b.innerHTML = `<span class="eq-val"></span><input type="range" min="-12" max="12" step="0.5"><span class="eq-f">${bandLabel(f)}</span>`;
    const r = $("input", b), v = $(".eq-val", b);
    const show = () => { const g = eq.gains[i]; v.textContent = (g > 0 ? "+" : "") + g; v.classList.toggle("pos", g > 0); v.classList.toggle("neg", g < 0); };
    r.value = eq.gains[i];
    show();
    r.addEventListener("input", () => {
      eq.gains[i] = Number(r.value);
      show();
      if (eq.preset) { eq.preset = ""; sel.value = ""; $(".eq-del", box).disabled = true; }
      sendEQ(which);
    });
    r.addEventListener("dblclick", () => { r.value = 0; r.dispatchEvent(new Event("input")); });
    bands.append(b);
  });

  on.addEventListener("change", () => { eq.enabled = on.checked; box.classList.toggle("eq-off", !eq.enabled); sendEQ(which, 0); });
  sel.addEventListener("change", () => {
    const p = eqProfiles.find((x) => x.name === sel.value);
    if (!p) { eq.preset = ""; return; }
    eq.preset = p.name;
    eq.gains = p.gains.slice();
    eq.enabled = true;
    renderEQ(box);
    sendEQ(which, 0);
  });
  $(".eq-flat", box).addEventListener("click", () => {
    eq.gains = eq.gains.map(() => 0);
    eq.preset = "Plano";
    renderEQ(box);
    sendEQ(which, 0);
  });
  $(".eq-save", box).addEventListener("click", () => {
    const name = $(".eq-name", box).value.trim();
    api.SaveEQProfile(name, eq.gains).then((list) => {
      eqProfiles = list;
      eq.preset = name;
      sendEQ(which, 0);
      $$(".eqbox").forEach(renderEQ);
      showOk(`Perfil «${name}» guardado.`);
    }).catch(showError);
  });
  $(".eq-del", box).addEventListener("click", () => {
    const name = eq.preset;
    api.DeleteEQProfile(name).then((list) => {
      eqProfiles = list;
      eq.preset = "";
      sendEQ(which, 0);
      $$(".eqbox").forEach(renderEQ);
      showOk(`Perfil «${name}» borrado.`);
    }).catch(showError);
  });
}

// ---------- nombre de la canción ----------
let playlist = { path: "", titles: [] };
let plIndex = -1;

function renderTitles() {
  const src = cfg.radio.titles.source;
  $$(".title-file").forEach((el) => el.classList.toggle("hidden", src === "manual"));
  $$(".title-manual").forEach((el) => el.classList.toggle("hidden", src !== "manual"));
  const hints = {
    manual: "Escriba el título o cargue una lista de canciones (.m3u, .pls o .txt) y envíelas con un clic o con «Enviar la siguiente».",
    jazler: "Automático con Jazler RadioStar 2: el emisor lee Exports\\NowOnAir.xml cada vez que Jazler cambia de tema y manda «Artista - Título». En cuñas y jingles manda el texto de arriba. No hace falta tocar nada en Jazler.",
    file: "Automático con otro programa (RadioDJ, ZaraRadio, mAirList…): configure en ese programa que escriba lo que suena en un archivo de texto y ponga aquí su ruta.",
  };
  $("#titles-hint").textContent = hints[src] || "";
}

function renderPlaylist() {
  const ol = $("#pl-list");
  ol.innerHTML = "";
  $("#pl-name").textContent = playlist.path ? `${playlist.path.split(/[\\/]/).pop()} · ${playlist.titles.length} canciones` : "Sin lista cargada";
  playlist.titles.forEach((t, i) => {
    const li = document.createElement("li");
    li.className = i === plIndex ? "current" : "";
    li.innerHTML = `<span class="pl-t"></span><button class="btn mini" type="button" title="Enviar este título">▶</button>`;
    $(".pl-t", li).textContent = t;
    const send = () => sendFromList(i);
    $("button", li).addEventListener("click", send);
    li.addEventListener("dblclick", send);
    ol.append(li);
  });
  $("#pl-next").disabled = !playlist.titles.length;
}

function sendFromList(i) {
  plIndex = i;
  renderPlaylist();
  const li = $("#pl-list").children[i];
  if (li) li.scrollIntoView({ block: "nearest" });
  api.SendSong(playlist.titles[i]).catch(showError);
}

function showNowPlaying(np) {
  lastNP = np;
  $("#np-title").textContent = np && np.title ? np.title : "—";
  const st = $("#np-status");
  if (!np) { st.textContent = ""; return; }
  st.textContent = np.status ? "⚠ " + np.status : np.title ? (np.sent ? "✓ en Icecast" : "pendiente: se envía con la radio al aire") : "";
  st.classList.toggle("bad", !!np.status);
}

function bindTitles() {
  $("#send-song").addEventListener("click", () => api.SendSong($("#song").value).then(() => { $("#song").value = ""; }).catch(showError));
  $("#song").addEventListener("keydown", (e) => { if (e.key === "Enter") $("#send-song").click(); });
  $("#pl-load").addEventListener("click", () => api.LoadPlaylist().then((pl) => {
    if (pl && pl.titles && pl.titles.length) { playlist = pl; plIndex = -1; renderPlaylist(); }
  }).catch(showError));
  $("#pl-next").addEventListener("click", () => { if (playlist.titles.length) sendFromList((plIndex + 1) % playlist.titles.length); });
  $("#detect-jazler").addEventListener("click", () => api.DetectJazler().then((p) => {
    cfg.radio.titles.file = p;
    $('[data-k="radio.titles.file"]').value = p;
    scheduleSave();
    showOk("Jazler encontrado: " + p);
  }).catch(showError));
  $('[data-k="radio.titles.source"]').addEventListener("change", () => {
    if (cfg.radio.titles.source === "jazler" && !cfg.radio.titles.file) {
      cfg.radio.titles.file = "C:\\Jazler RadioStar 2\\Exports\\NowOnAir.xml";
      $('[data-k="radio.titles.file"]').value = cfg.radio.titles.file;
    }
    renderTitles();
  });
}

// setGain manda la ganancia al aire (agrupando los movimientos del deslizador).
const gainTimers = {};
function setGain(which, g) {
  cfg[which].gain_db = g;
  clearTimeout(gainTimers[which]);
  gainTimers[which] = setTimeout(() => api.SetGain(which, g).catch(showError), 120);
}

// ---------- sub-apartados ----------
function showSub(tab, sub) {
  showTab(tab);
  const sec = $("#tab-" + tab);
  $$(".subtab", sec).forEach((b) => b.classList.toggle("active", b.dataset.sub === sub));
  $$(".subpanel", sec).forEach((p) => p.classList.toggle("active", p.dataset.sub === sub));
  // El aviso de «Aplicar» no hace falta en el resumen ni en mejoras/canciones (se aplican al aire).
  const bar = $(".applybar", sec);
  if (bar) bar.classList.toggle("hidden-sub", ["mejoras", "canciones"].includes(sub));
  $("main").scrollTop = 0;
}

function bindSubtabs() {
  $$(".panel").forEach((sec) => {
    const tab = sec.id.replace("tab-", "");
    $$(".subtab", sec).forEach((b) => b.addEventListener("click", () => showSub(tab, b.dataset.sub)));
  });
  // Accesos directos del resumen («Cambiar →»).
  document.addEventListener("click", (e) => {
    const a = e.target.closest("[data-goto]");
    if (!a) return;
    const [tab, sub] = a.dataset.goto.split(".");
    showSub(tab, sub);
  });
}

// ---------- resumen ----------
let lastNP = null;

function esc(s) {
  return String(s ?? "").replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
}

function sourceText(role, s) {
  const k = (KINDS[role] || []).find(([id]) => id === s.kind);
  const label = k ? k[1].replace(/^[^A-Za-zÁÉÍÓÚáéíóúÑñ(]+/, "") : s.kind; // sin el emoji
  if (s.kind === "dshow") return s.name ? s.name : "⚠ sin elegir";
  if (s.kind === "loopback") return "Audio de la PC · " + (s.name || "salida predeterminada");
  if (s.kind === "url") return "Dirección: " + (s.name || "⚠ sin escribir");
  if (s.kind === "rtsp") return "Cámara RTSP: " + (s.name ? hidePass(s.name) : "⚠ sin escribir");
  return label;
}

// tvAudioText es el audio de la TV para el resumen, según el modo.
function tvAudioText(t) {
  const short = { radio: "audio de la radio", icecast: "radio desde el servidor" };
  const other = short[t.audio.kind] || sourceText("tvaudio", t.audio);
  if (!NET_VIDEO(t.video.kind)) return other;
  switch (t.audio_mode) {
    case "video": return "el que trae el video · si no trae: " + other;
    case "both": return `video (${fmtDB(t.video_mix_db)}) + ${other} (${fmtDB(t.other_mix_db)})`;
    default: return other + " (el del video no sale)";
  }
}
function fmtDB(v) { v = Number(v) || 0; return (v > 0 ? "+" : "") + v + " dB"; }

function eqText(eq) {
  if (!eq || !eq.enabled) return "apagado";
  return eq.preset ? "perfil «" + eq.preset + "»" : "ajuste manual";
}

function card(title, goto, rows, cls = "") {
  return `<div class="sum-card ${cls}">
    <div class="sum-head"><b>${title}</b>${goto ? `<button class="link" data-goto="${goto}">Cambiar →</button>` : ""}</div>
    ${rows.map(([k, v, warn]) => `<div class="sum-row${warn ? " warn" : ""}"><span>${k}</span><span>${v}</span></div>`).join("")}
  </div>`;
}

function statusRows(name) {
  const st = lastStatus[name] || { state: "detenido" };
  const live = st.state === "en_vivo" && st.live_since && !st.live_since.startsWith("0001");
  const rows = [["Estado", `<span class="pill" data-state="${st.state}">${esc(STATE_LABEL[st.state] || st.state)}</span>`]];
  if (live) rows.push(["Al aire desde hace", fmtTime((Date.now() - Date.parse(st.live_since)) / 1000)],
    ["Envía", Math.round(st.bitrate_kbps || 0) + " kbps" + (name === "tv" && st.fps ? ` · ${st.fps.toFixed(0)} fps` : "")]);
  if (st.message) rows.push(["Aviso", esc(st.message), true]);
  rows.push(["Emitir al pulsar ▶ Iniciar", cfg[name].enabled ? "sí" : "no (desmarcado)"]);
  return rows;
}

function renderSummary() {
  if (!cfg) return;
  const r = cfg.radio, t = cfg.tv;
  const codec = (state.codecs.find((c) => c.id === r.codec) || { label: r.codec }).label;
  const rbox = $('[data-summary="radio"]');
  if (rbox) {
    rbox.innerHTML =
      card("Estado", "", statusRows("radio"), "wide") +
      card("Entrada", "radio.entrada", [["Audio", esc(sourceText("audio", r.input)), r.input.kind === "dshow" && !r.input.name]]) +
      card("Stream", "radio.stream", [
        ["Formato", `${esc(codec)} · ${r.bitrate_kbps} kbps`],
        ["Audio", `${r.codec === "opus" ? 48000 : r.sample_rate} Hz · ${r.channels === 1 ? "mono" : "estéreo"}`],
        ["Servidor", esc(`${r.host}:${r.port}${r.mount}`)],
        ["Contraseña", r.password ? "✓ guardada" : "⚠ falta", !r.password],
      ]) +
      card("Mejoras", "radio.mejoras", [
        ["Ganancia", (r.gain_db > 0 ? "+" : "") + (r.gain_db || 0) + " dB"],
        ["MEJORA", r.enhance ? "✦ activa" : "apagada"],
        ["Ecualizador", esc(eqText(r.eq))],
      ]) +
      card("Canciones", "radio.canciones", [
        ["Nombre desde", { manual: "a mano / lista", jazler: "Jazler RadioStar 2", file: "archivo de texto" }[r.titles.source] || r.titles.source],
        ["Sonando", lastNP && lastNP.title ? esc(lastNP.title) : "—"],
        ...(lastNP && lastNP.status ? [["Aviso", esc(lastNP.status), true]] : []),
      ]);
  }
  const tbox = $('[data-summary="tv"]');
  if (tbox) {
    const port = t.protocol === "srt" ? t.srt_port : t.rtmp_port;
    tbox.innerHTML =
      card("Estado", "", statusRows("tv"), "wide") +
      card("Entradas", "tv.entradas", [
        ["Video", esc(sourceText("video", t.video)), t.video.kind === "dshow" && !t.video.name],
        ["Audio", esc(tvAudioText(t))],
      ]) +
      card("Stream", "tv.stream", [
        ["Calidad", `${t.width}×${t.height} · ${t.fps} fps`],
        ["Bitrate", `${t.video_kbps} kbps video · ${t.audio_kbps} kbps audio`],
        ["Servidor", esc(`${t.protocol}://${t.host}:${port}/${t.path}`)],
        ["Clave", t.password ? "✓ guardada" : "⚠ falta", !t.password],
      ]) +
      card("Mejoras del audio", "tv.mejoras", [
        ["Ganancia", (t.gain_db > 0 ? "+" : "") + (t.gain_db || 0) + " dB"],
        ["MEJORA", t.enhance ? "✦ activa" : "apagada"],
        ["Ecualizador", esc(eqText(t.eq))],
      ]);
  }
}

// subForError elige el apartado según el mensaje de error de la validación.
function subForError(which, msg) {
  const m = msg.toLowerCase();
  if (/(micr|entrada|c[aá]mara|video|direcci[oó]n del video|fuente|mezcla|modo de audio|audio de la radio|radio desde)/.test(m)) return which === "tv" ? "entradas" : "entrada";
  if (/(banda|ecualiz|ganancia)/.test(m)) return "mejoras";
  return "stream";
}

// ---------- licencia, contacto y actualización ----------
// El programa solo funciona autenticado (dominio + token de Admin → Estudio (PC)).
// Sin autenticar se ve todo pero bloqueado: cualquier intento abre el aviso de licencia.
// Con una actualización publicada, cualquier opción abre el aviso para actualizar
// (la emisión sigue; «Detener» siempre funciona).
let license = { authenticated: true };
let update = { available: false };
let autoAuthShown = false;

const isLocked = () => !license || !license.authenticated;
const needsUpdate = () => !isLocked() && update && update.available;

function openModal(id) {
  $$(".modal-bg").forEach((m) => m.classList.toggle("hidden", m.id !== id));
  const m = $("#" + id);
  const f = $("input:not([type=hidden]), button.primary", m);
  if (f) setTimeout(() => f.focus(), 30);
}
function closeModals() { $$(".modal-bg").forEach((m) => m.classList.add("hidden")); }
const modalOpen = () => $$(".modal-bg").some((m) => !m.classList.contains("hidden"));

function waLink(phone) {
  let d = String(phone || "").replace(/\D/g, "");
  if (d.length === 9) d = "51" + d; // celular de Perú sin código de país
  return "https://wa.me/" + d;
}

function renderLicense() {
  document.body.classList.toggle("locked", isLocked());
  document.body.classList.toggle("needs-update", needsUpdate());
  const p = (license && license.provider) || { phone: "9488933154", url: "https://sehuacho.com/radio" };
  $$(".prov-phone").forEach((e) => (e.textContent = p.phone));
  $$(".prov-url").forEach((e) => (e.textContent = p.url.replace(/^https?:\/\//, "")));
  // Aviso superior.
  const bar = $("#lockbar");
  if (isLocked()) {
    bar.className = "lockbar locked";
    $("#lockbar-text").textContent = "🔒 Programa no autenticado" + (license.reason ? ": " + license.reason : ". Ingrese el dominio y el token de su radio.");
    $("#lockbar-update").classList.add("hidden");
    $("#lockbar-auth").classList.remove("hidden");
    $("#lockbar-contact").classList.remove("hidden");
  } else if (needsUpdate()) {
    bar.className = "lockbar update";
    $("#lockbar-text").textContent = `⬆ Actualización obligatoria: versión ${update.version} (tiene ${update.current}). La emisión sigue al aire.`;
    $("#lockbar-update").classList.remove("hidden");
    $("#lockbar-auth").classList.add("hidden");
    $("#lockbar-contact").classList.add("hidden");
  } else if (license.offline) {
    bar.className = "lockbar offline";
    $("#lockbar-text").textContent = "⚠ " + license.reason;
    ["#lockbar-update", "#lockbar-auth", "#lockbar-contact"].forEach((s) => $(s).classList.add("hidden"));
  } else {
    bar.className = "lockbar hidden";
  }
  // Ajustes → Programa.
  const box = $("#lic-status");
  if (box) {
    const s = license.station || {};
    box.innerHTML = isLocked()
      ? `<span class="pill" data-state="reconectando">No autenticado</span> <span class="muted">${esc(license.reason || "")}</span>`
      : `<span class="pill" data-state="en_vivo">✓ Autenticado</span> ${esc(s.name || "")} ${esc(s.frequency || "")} · ${esc(license.base || "")}` +
        (license.offline ? ` <span class="muted">(sin conexión: válido hasta ${new Date(license.valid_until).toLocaleString()})</span>` : "");
    $("#upd-status").textContent = update.available
      ? `Versión ${update.version} disponible (tiene ${update.current}).`
      : `Versión ${state.version}: está al día.`;
  }
  // Modal de actualización.
  $("#upd-version").textContent = update.version || "";
  $("#upd-current").textContent = update.current || state.version;
  $("#upd-notes").textContent = update.notes || "";
  $("#upd-notes").classList.toggle("hidden", !update.notes);
  if (isLocked() && !autoAuthShown && !modalOpen() && license.checked_at && !license.checked_at.startsWith("0001")) {
    autoAuthShown = true; // al abrir sin autenticar: pedir dominio y token
    showAuth();
  }
}

function showAuth() {
  $("#auth-url").value = (cfg && cfg.panel && cfg.panel.url) || "";
  $("#auth-token").value = (cfg && cfg.panel && cfg.panel.token) || "";
  $("#auth-err").textContent = "";
  openModal("m-auth");
}

// gate intercepta clics y foco: sin licencia → aviso de licencia; con
// actualización pendiente → aviso para actualizar (salvo «Detener»).
function gate(e) {
  const t = e.target;
  if (!(t instanceof Element) || t.closest(".modal-bg, .lockbar")) return;
  if (!isLocked() && !needsUpdate()) return;
  if (needsUpdate() && t.closest("[data-stop], #stop-all")) return; // detener siempre se puede
  if (e.type === "focusin" && !t.matches("input, select, textarea, button")) return;
  e.preventDefault();
  e.stopPropagation();
  if (e.type === "focusin") t.blur();
  if (isLocked()) openModal("m-license");
  else openModal("m-update");
}

function bindLicense() {
  document.addEventListener("click", gate, true);
  document.addEventListener("focusin", gate, true);
  $$(".modal-bg [data-close]").forEach((b) => b.addEventListener("click", closeModals));
  $$(".open-auth").forEach((b) => b.addEventListener("click", showAuth));
  $$(".open-contact").forEach((b) => b.addEventListener("click", () => openModal("m-license")));
  $$(".open-update").forEach((b) => b.addEventListener("click", () => openModal("m-update")));
  $$(".prov-wa").forEach((b) => b.addEventListener("click", () => api.OpenURL(waLink(license.provider.phone)).catch(showError)));
  $$(".prov-web").forEach((b) => b.addEventListener("click", () => api.OpenURL(license.provider.url).catch(showError)));
  $$(".prov-copy").forEach((b) => b.addEventListener("click", () => {
    try { navigator.clipboard.writeText(license.provider.phone); b.textContent = "✓ Copiado"; } catch (_) { /* sin portapapeles */ }
  }));
  $("#auth-go").addEventListener("click", () => {
    const btn = $("#auth-go");
    btn.disabled = true;
    $("#auth-err").textContent = "Comprobando…";
    api.Authenticate($("#auth-url").value, $("#auth-token").value).then((st) => {
      license = st;
      cfg.panel.url = st.base; cfg.panel.token = $("#auth-token").value;
      $$('[data-k="panel.url"]').forEach((e) => (e.value = cfg.panel.url));
      $$('[data-k="panel.token"]').forEach((e) => (e.value = cfg.panel.token));
      closeModals();
      renderLicense();
      showOk(`Programa autenticado: ${st.station.name} ${st.station.frequency}.`);
    }).catch((e) => { $("#auth-err").textContent = String(e && e.message ? e.message : e); })
      .finally(() => (btn.disabled = false));
  });
  $("#auth-token").addEventListener("keydown", (e) => { if (e.key === "Enter") $("#auth-go").click(); });
  $("#upd-go").addEventListener("click", () => {
    $("#upd-go").disabled = true;
    $("#upd-err").textContent = "";
    $("#upd-progress").classList.remove("hidden");
    api.ApplyUpdate().catch((e) => {
      $("#upd-err").textContent = String(e && e.message ? e.message : e);
      $("#upd-go").disabled = false;
      $("#upd-progress").classList.add("hidden");
    });
  });
  $("#lic-check").addEventListener("click", () => api.CheckLicense().then((st) => {
    license = st; renderLicense();
    showOk(st.authenticated ? "Licencia verificada." : "No autenticado: " + st.reason);
  }).catch(showError));
  $("#cfg-restore").addEventListener("click", () => {
    if (!confirm("¿Reemplazar la configuración de esta PC por la copia guardada en el servidor? (Se conservan las contraseñas, el token y el arranque.)")) return;
    api.RestoreConfig().then((r) => {
      cfg = r.config; render();
      showOk("Configuración restaurada (copia del " + new Date(r.saved_at).toLocaleString() + ").");
    }).catch(showError);
  });
  $("#panel-auth").addEventListener("click", () => {
    api.Authenticate(cfg.panel.url, cfg.panel.token).then((st) => {
      license = st; renderLicense(); showOk(`Programa autenticado: ${st.station.name} ${st.station.frequency}.`);
    }).catch(showError);
  });
  on("licencia", (d) => { license = d.license; update = d.update; renderLicense(); });
  on("actualizacion", (d) => {
    $("#upd-progress").classList.remove("hidden");
    $("#upd-bar").style.width = (d.pct || 0) + "%";
    $("#upd-stage").textContent = { descargando: `Descargando… ${d.pct}%`, instalando: "Instalando…",
      reiniciando: "Reiniciando el programa: vuelve en unos segundos y retoma la emisión." }[d.stage] || d.stage;
  });
}
