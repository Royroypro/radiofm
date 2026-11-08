/* =====================================================================
   RadioClub25 — lógica de la web pública (sin dependencias, salvo hls.js
   que se carga solo en la portada).

   Módulos:
     1. Utilidades (almacenamiento seguro, beacon de eventos)
     2. Menú móvil
     3. Medición de banners (impresión al aparecer en pantalla)
     4. Publicidad previa (cuña de audio / pre-roll de video)
     5. Radio (Icecast, <audio>)
     6. TV (HLS de MediaMTX, <video> + hls.js)
     7. Estado en vivo (consulta /api/status cada 15 s)
     8. Espectro de audio animado del hero (simulado o real con Web Audio)

   La configuración llega del servidor en window.RC (layouts/public.html):
     { radioURL, tvURL, radioEnabled, tvEnabled, slogan, prerollIntervalMin }
   ===================================================================== */
(function () {
  'use strict';

  var RC = window.RC || {};
  var $ = function (sel, root) { return (root || document).querySelector(sel); };
  var $$ = function (sel, root) { return Array.prototype.slice.call((root || document).querySelectorAll(sel)); };

  /* ---------- 1. Utilidades ---------- */

  // localStorage puede fallar (modo privado, cookies bloqueadas): siempre con try/catch.
  var store = {
    get: function (k) { try { return window.localStorage.getItem(k); } catch (e) { return null; } },
    set: function (k, v) { try { window.localStorage.setItem(k, v); } catch (e) { /* sin almacenamiento */ } }
  };

  // Envía un evento de anuncio (impression, complete, skip) sin bloquear la página.
  function adEvent(adId, type) {
    var body = JSON.stringify({ ad_id: Number(adId), type: type });
    try {
      if (navigator.sendBeacon && navigator.sendBeacon('/api/ads/event', new Blob([body], { type: 'text/plain' }))) return;
    } catch (e) { /* cae a fetch */ }
    fetch('/api/ads/event', { method: 'POST', body: body, keepalive: true, headers: { 'Content-Type': 'text/plain' } })
      .catch(function () {});
  }

  function setText(sel, text) { $$(sel).forEach(function (el) { el.textContent = text; }); }

  /* ---------- 2. Menú móvil ---------- */

  var navToggle = $('[data-nav-toggle]');
  var nav = $('[data-nav]');
  if (navToggle && nav) {
    navToggle.addEventListener('click', function () {
      var open = nav.classList.toggle('open');
      navToggle.setAttribute('aria-expanded', open ? 'true' : 'false');
    });
  }

  /* ---------- 3. Medición de banners ---------- */

  var banners = $$('.ad-banner[data-ad-id]');
  if (banners.length && 'IntersectionObserver' in window) {
    var seen = {};
    var io = new IntersectionObserver(function (entries) {
      entries.forEach(function (e) {
        var id = e.target.getAttribute('data-ad-id');
        if (e.isIntersecting && !seen[id]) {
          seen[id] = true;
          adEvent(id, 'impression');
          io.unobserve(e.target);
        }
      });
    }, { threshold: 0.5 });
    banners.forEach(function (b) { io.observe(b); });
  }

  /* ---------- 4. Publicidad previa ---------- */

  // ¿Toca mostrar publicidad? Respeta el intervalo configurado en el panel.
  function adDue(kind) {
    var last = Number(store.get('rc_ad_' + kind) || 0);
    var interval = Number(RC.prerollIntervalMin || 0) * 60000;
    return Date.now() - last >= interval;
  }

  // Pide un anuncio al servidor. Resuelve null si no hay o si falla.
  function fetchAd(kind) {
    return fetch('/api/ads/preroll?kind=' + kind, { cache: 'no-store' })
      .then(function (r) { return r.status === 200 ? r.json() : null; })
      .catch(function () { return null; });
  }

  // Reproduce un anuncio en media (audio o video) y llama a done() al terminar,
  // al saltarlo o si falla. ui = { box, link, skip } (elementos de la interfaz).
  function runAd(media, ad, ui, done) {
    var finished = false;
    var counted = false;
    var timer = null;
    store.set('rc_ad_' + (media.tagName === 'VIDEO' ? 'video' : 'audio'), String(Date.now()));

    function finish(evt) {
      if (finished) return;
      finished = true;
      clearInterval(timer);
      clearTimeout(watchdog);
      media.removeEventListener('ended', onEnded);
      media.removeEventListener('error', onError);
      media.removeEventListener('playing', onPlaying);
      if (evt && counted) adEvent(ad.id, evt);
      ui.box.hidden = true;
      media.pause();
      media.removeAttribute('src');
      media.load();
      done();
    }
    function onEnded() { finish('complete'); }
    function onError() { finish(null); }
    function onPlaying() {
      if (!counted) { counted = true; adEvent(ad.id, 'impression'); }
    }

    ui.box.hidden = false;
    if (ui.link) {
      ui.link.hidden = !ad.has_link;
      if (ad.has_link) ui.link.href = ad.click_url;
    }
    if (ui.skip) {
      var left = Number(ad.skip_after || 0);
      ui.skip.hidden = left <= 0;
      ui.skip.disabled = true;
      ui.skip.textContent = 'Saltar en ' + left + ' s';
      timer = setInterval(function () {
        left -= 1;
        if (left <= 0) {
          clearInterval(timer);
          ui.skip.disabled = false;
          ui.skip.textContent = 'Saltar anuncio';
        } else {
          ui.skip.textContent = 'Saltar en ' + left + ' s';
        }
      }, 1000);
      ui.skip.onclick = function () { finish('skip'); };
    }

    // Si el anuncio no arranca en 8 s, no hacemos esperar al oyente.
    var watchdog = setTimeout(function () { if (!counted) finish(null); }, 8000);
    media.addEventListener('ended', onEnded);
    media.addEventListener('error', onError);
    media.addEventListener('playing', onPlaying);
    media.src = ad.url;
    var p = media.play();
    if (p && p.catch) p.catch(function () { finish(null); });
  }

  /* ---------- 5. Radio ---------- */

  var radio = (function () {
    var audio = $('#radio-audio');
    if (!audio || !RC.radioEnabled) {
      $$('[data-radio-toggle]').forEach(function (b) { b.hidden = true; });
      return null;
    }
    var roots = $$('[data-radio-root]');
    var state = 'idle'; // idle | ad | playing
    var retries = 0;
    var retryTimer = null;
    var spot = {
      box: $('[data-spot]'), link: $('[data-spot-link]'), skip: $('[data-spot-skip]')
    };

    var vol = $('[data-radio-volume]');
    var savedVol = Number(store.get('rc_volume'));
    if (!isNaN(savedVol) && store.get('rc_volume') !== null) audio.volume = savedVol;
    if (vol) {
      vol.value = audio.volume;
      vol.addEventListener('input', function () { audio.volume = Number(vol.value); store.set('rc_volume', vol.value); });
    }

    function setPlaying(on) {
      roots.forEach(function (r) { r.classList.toggle('is-playing', on); });
      $$('[data-radio-toggle]').forEach(function (b) { b.setAttribute('aria-pressed', on ? 'true' : 'false'); });
    }
    function msg(t) { setText('[data-radio-msg]', t || ''); }

    function startStream() {
      state = 'playing';
      var url = RC.radioURL || audio.getAttribute('data-src');
      audio.src = url + (url.indexOf('?') >= 0 ? '&' : '?') + 't=' + Date.now(); // evita cachés intermedias
      var p = audio.play();
      if (p && p.catch) p.catch(function () { msg('Toca reproducir de nuevo para escuchar.'); stop(); });
      setPlaying(true);
      if ('mediaSession' in navigator && window.MediaMetadata) {
        navigator.mediaSession.metadata = new MediaMetadata({ title: document.title, artist: RC.slogan || '' });
      }
    }

    function play() {
      if (state !== 'idle') return;
      if (tv) tv.pause();
      msg('');
      retries = 0;
      setPlaying(true);
      if (spot.box && adDue('audio')) {
        state = 'ad';
        fetchAd('audio').then(function (ad) {
          if (state !== 'ad') return;
          if (ad) runAd(audio, ad, spot, startStream); else startStream();
        });
      } else {
        startStream();
      }
    }

    function stop() {
      state = 'idle';
      clearTimeout(retryTimer);
      audio.pause();
      audio.removeAttribute('src');
      audio.load(); // corta la descarga del stream
      setPlaying(false);
      if (spot.box) spot.box.hidden = true;
    }

    audio.addEventListener('playing', function () { if (state === 'playing') { retries = 0; msg(''); } });
    audio.addEventListener('error', function () {
      if (state !== 'playing') return;
      if (retries >= 5) { msg('La radio no está disponible en este momento.'); stop(); return; }
      retries += 1;
      msg('Reconectando…');
      clearTimeout(retryTimer);
      retryTimer = setTimeout(function () { if (state === 'playing') startStream(); }, 3000);
    });

    $$('[data-radio-toggle]').forEach(function (b) {
      b.addEventListener('click', function () { if (state === 'idle') play(); else stop(); });
    });

    return { pause: function () { if (state !== 'idle') stop(); } };
  })();

  /* ---------- 6. TV ---------- */

  var tv = (function () {
    var video = $('#tv-video');
    if (!video || !RC.tvEnabled) return null;
    var overlay = $('[data-tv-overlay]');
    var playBtn = $('[data-tv-play]');
    var pre = { box: $('[data-preroll]'), link: $('[data-preroll-link]'), skip: $('[data-preroll-skip]') };
    var hls = null;
    var state = 'idle'; // idle | ad | live

    function msg(t) { setText('[data-tv-msg]', t || ''); }

    function destroyHls() { if (hls) { hls.destroy(); hls = null; } }

    function startLive() {
      state = 'live';
      video.controls = true;
      var url = RC.tvURL || video.getAttribute('data-src');
      destroyHls();
      if (window.Hls && window.Hls.isSupported()) {
        hls = new window.Hls({ lowLatencyMode: true, liveDurationInfinity: true, backBufferLength: 30 });
        hls.loadSource(url);
        hls.attachMedia(video);
        hls.on(window.Hls.Events.MANIFEST_PARSED, function () { video.play().catch(function () {}); });
        hls.on(window.Hls.Events.ERROR, function (_e, data) {
          if (!data.fatal) return;
          if (data.type === window.Hls.ErrorTypes.NETWORK_ERROR) {
            msg('Señal interrumpida, reconectando…');
            setTimeout(function () { if (hls) hls.startLoad(); }, 3000);
          } else if (data.type === window.Hls.ErrorTypes.MEDIA_ERROR) {
            hls.recoverMediaError();
          } else {
            msg('No se pudo reproducir la TV.');
            pause();
          }
        });
      } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
        video.src = url; // Safari / iOS reproducen HLS de forma nativa
        video.play().catch(function () {});
      } else {
        msg('Su navegador no puede reproducir la TV en vivo.');
        pause();
      }
    }

    function play() {
      if (state !== 'idle') return;
      if (radio) radio.pause();
      msg('');
      overlay.hidden = true;
      if (pre.box && adDue('video')) {
        state = 'ad';
        video.controls = false;
        fetchAd('video').then(function (ad) {
          if (state !== 'ad') return;
          if (ad) runAd(video, ad, pre, startLive); else startLive();
        });
      } else {
        startLive();
      }
    }

    function pause() {
      state = 'idle';
      destroyHls();
      video.pause();
      video.removeAttribute('src');
      video.load();
      video.controls = true;
      overlay.hidden = false;
    }

    video.addEventListener('playing', function () { if (state === 'live') msg(''); });
    // Si el usuario sube el volumen del video, que se calle la radio.
    video.addEventListener('volumechange', function () { if (state === 'live' && !video.muted && radio) radio.pause(); });
    if (playBtn) playBtn.addEventListener('click', play);

    return {
      pause: function () { if (state !== 'idle') pause(); },
      isIdle: function () { return state === 'idle'; },
      setOnline: function (online) {
        if (!overlay || !playBtn) return;
        playBtn.hidden = !online;
        setText('[data-tv-overlay-title]', online ? 'Toca para ver la señal en vivo' : 'La TV está fuera del aire');
        setText('[data-tv-overlay-sub]', online ? '' : 'Vuelve pronto o escucha la radio mientras tanto.');
      }
    };
  })();

  /* ---------- 7. Estado en vivo ---------- */

  function badge(sel, online) {
    $$(sel).forEach(function (b) {
      b.classList.toggle('on', online);
      b.textContent = online ? 'En vivo' : 'Fuera del aire';
    });
  }

  function refreshStatus() {
    fetch('/api/status', { cache: 'no-store' })
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (s) {
        if (!s) return;
        var pill = $('#live-pill');
        if (pill) pill.classList.toggle('on', !!(s.radio.online || s.tv.online));
        badge('[data-radio-badge]', s.radio.online);
        badge('[data-tv-badge]', s.tv.online);
        setText('[data-listeners]', String(s.radio.listeners || 0));
        setText('[data-viewers]', String(s.tv.viewers || 0));
        setText('[data-now-title]', s.radio.title || RC.slogan || '');
        var onAir = $('[data-on-air]');
        if (onAir) {
          onAir.hidden = !s.on_air;
          if (s.on_air) {
            setText('[data-on-air-title]', s.on_air.title);
            setText('[data-on-air-host]', (s.on_air.host ? s.on_air.host + ' · ' : '') + s.on_air.start + '–' + s.on_air.end);
            var img = $('[data-on-air-img]');
            if (img && s.on_air.image) img.src = s.on_air.image;
          }
        }
        if (tv && tv.isIdle()) tv.setOnline(s.tv.online);
      })
      .catch(function () { /* sin conexión: se reintenta en el siguiente ciclo */ });
  }

  refreshStatus();
  setInterval(function () { if (!document.hidden) refreshStatus(); }, 15000);

  /* ---------- 8. Espectro de audio del hero ----------
     Barras simétricas desde el centro, en los colores de la marca.
     - En reposo: ritmo "de fiesta" simulado (128 BPM, bombo marcado).
     - Con la radio sonando: datos reales de un AnalyserNode (Web Audio).
       Solo si el stream es del mismo origen (/radio vía nginx); con otro
       origen sin CORS el navegador silenciaría el audio.
     - Se pausa fuera de pantalla o con la pestaña oculta; con
       prefers-reduced-motion se dibuja un solo cuadro estático. */
  (function spectrum() {
    var canvas = $('[data-spectrum]');
    if (!canvas || !canvas.getContext) return;
    var ctx = canvas.getContext('2d');
    var audio = $('#radio-audio');
    var reduced = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    var css = getComputedStyle(document.documentElement);
    var colors = ['--red', '--orange', '--sky'].map(function (v) { return (css.getPropertyValue(v) || '').trim() || '#FF1E1E'; });

    var W = 0, H = 0, dpr = 1, bars = 48, levels = [], peaks = [];
    var analyser = null, freq = null, actx = null, visible = true, raf = 0;

    function resize() {
      dpr = Math.min(window.devicePixelRatio || 1, 2);
      W = canvas.clientWidth; H = canvas.clientHeight;
      canvas.width = Math.round(W * dpr); canvas.height = Math.round(H * dpr);
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      bars = Math.max(16, Math.min(72, Math.round(W / 22)));
      if (bars % 2) bars += 1;
      levels = new Array(bars).fill(0); peaks = new Array(bars).fill(0);
    }

    // Conecta el analizador al <audio> de la radio la primera vez que suena.
    function attach() {
      if (analyser || !audio || !window.AudioContext && !window.webkitAudioContext) return;
      var url = (window.RC && RC.radioURL) || '';
      if (url && !/^\//.test(url) && url.indexOf(location.origin) !== 0) return; // otro origen: no tocar
      try {
        actx = new (window.AudioContext || window.webkitAudioContext)();
        var src = actx.createMediaElementSource(audio);
        analyser = actx.createAnalyser();
        analyser.fftSize = 256;
        analyser.smoothingTimeConstant = 0.78;
        src.connect(analyser);
        analyser.connect(actx.destination);
        freq = new Uint8Array(analyser.frequencyBinCount);
      } catch (e) { analyser = null; }
    }
    if (audio) {
      audio.addEventListener('playing', function () {
        attach();
        if (actx && actx.state === 'suspended') actx.resume().catch(function () {});
      });
    }

    function playing() { return audio && !audio.paused && !audio.ended && audio.currentTime > 0; }

    // Valor 0..1 de la barra i (mitad izquierda; se refleja a la derecha).
    function simulated(i, half, t) {
      var x = i / half;                          // 0 = centro (graves) … 1 = extremos (agudos)
      var beat = (t * 128 / 60) % 1;             // posición dentro del tiempo
      var kick = Math.exp(-beat * 7);            // golpe de bombo
      var off = Math.exp(-((beat + 0.5) % 1) * 9) * 0.45; // contratiempo (hi-hat)
      var wobble = 0.5 + 0.5 * Math.sin(t * 2.3 + i * 0.55) * Math.sin(t * 1.1 + i * 0.21);
      var bass = (1 - x) * (0.35 + 0.65 * kick);
      var highs = x * (0.25 + off) * (0.6 + 0.4 * Math.sin(t * 9 + i));
      return Math.min(1, 0.12 + bass * 0.85 + highs * 0.7 + wobble * 0.25);
    }

    function draw(now) {
      var t = now / 1000, half = bars / 2, live = analyser && playing();
      if (live) analyser.getByteFrequencyData(freq);
      ctx.clearRect(0, 0, W, H);
      var gap = Math.max(2, W / bars * 0.28), bw = W / bars - gap;
      var grad = ctx.createLinearGradient(0, H, 0, 0);
      grad.addColorStop(0, colors[0]); grad.addColorStop(0.55, colors[1]); grad.addColorStop(1, colors[2]);

      for (var i = 0; i < half; i++) {
        var target;
        if (live) {
          var bin = Math.floor(Math.pow(i / half, 1.6) * (freq.length * 0.75)); // escala aprox. logarítmica
          target = Math.pow(freq[bin] / 255, 1.3);
        } else {
          target = simulated(i, half, t) * (playing() ? 1 : 0.75);
        }
        var cur = levels[i];
        cur += (target - cur) * (target > cur ? 0.55 : 0.12); // sube rápido, baja suave
        levels[i] = cur;
        peaks[i] = Math.max(peaks[i] - 0.008, cur);

        var h = Math.max(4, cur * H * 0.95), ph = peaks[i] * H * 0.95;
        [half - 1 - i, half + i].forEach(function (k) {   // simétrico desde el centro
          var x = k * (bw + gap) + gap / 2;
          ctx.fillStyle = grad;
          ctx.fillRect(x, H - h, bw, h);
          ctx.fillStyle = 'rgba(255,255,255,.9)';
          ctx.fillRect(x, H - ph - 6, bw, 3);          // tapa del pico
        });
      }
      if (!reduced && visible && !document.hidden) raf = requestAnimationFrame(draw);
      else raf = 0;
    }

    function start() { if (!raf && !reduced) raf = requestAnimationFrame(draw); }

    resize();
    if ('ResizeObserver' in window) new ResizeObserver(function () { resize(); if (reduced) draw(0); }).observe(canvas);
    else window.addEventListener('resize', resize);
    if ('IntersectionObserver' in window) {
      new IntersectionObserver(function (e) { visible = e[0].isIntersecting; if (visible) start(); }).observe(canvas);
    }
    document.addEventListener('visibilitychange', function () { if (!document.hidden) start(); });
    if (reduced) draw(4000); else start();
  })();
})();
