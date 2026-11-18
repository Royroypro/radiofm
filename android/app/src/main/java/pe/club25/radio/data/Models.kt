package pe.club25.radio.data

import org.json.JSONObject

/*
 * Modelos de la API del servidor radiofm (ver docs/api.md):
 * GET /api/app/config (marca, frecuencias, TV, ajustes de la app) y
 * GET /api/status (estado en vivo). Las URL se guardan ya resueltas contra la URL base.
 */

/**
 * Colores de la app (Admin → Marca y contacto → App Android → «Colores de la app»), como ARGB.
 * Son aparte de los de la web. Los valores por defecto son los mismos del panel.
 */
data class AppColors(
    val bg: Long = 0xFF3A0A4B,     // fondo, arriba del degradado
    val bg2: Long = 0xFF1E0526,    // fondo, abajo del degradado
    val card: Long = 0xFF6A1B9A,   // tarjetas
    val accent: Long = 0xFFFF961E, // avisos, «Señal externa»
    val play: Long = 0xFFFF1E1E,   // botón de play
    val text: Long = 0xFFFFFFFF,   // textos
)

/** Una frecuencia activa: un reproductor de radio en la app. */
data class Station(
    val id: Long,
    val label: String,     // "93.3 FM"
    val city: String,      // "Huacho"
    val listenUrl: String, // URL completa del audio
    val external: Boolean, // de otro servidor: no se sabe si está al aire
)

/** Un enlace de contacto o red social para mostrar como botón. */
data class LinkItem(val label: String, val url: String)

data class AppConfig(
    val name: String,
    val slogan: String,
    val logoUrl: String,
    val iconUrl: String,
    val colors: AppColors,
    val radioEnabled: Boolean,
    val stations: List<Station>,
    val tvEnabled: Boolean,
    val tvHlsUrl: String,
    val links: List<LinkItem>,
    val notice: String,
    val noticeUrl: String,
    val minVersion: Int,
    val playStoreUrl: String,
    val apkUrl: String,        // APK para descarga directa (si la app no está en Play o como alternativa)
    val privacyUrl: String,
    val statusUrl: String,
) {
    companion object {
        private val hexColor = Regex("^#[0-9A-Fa-f]{6}$")

        /** Lee el JSON de /api/app/config; [resolve] convierte URL relativas en completas. */
        fun parse(json: String, resolve: (String) -> String): AppConfig {
            val o = JSONObject(json)
            val colors = o.optJSONObject("app_colors") // los de la app, no los de la web ("colors")
            val def = AppColors()
            fun color(key: String, fallback: Long): Long = parseColor(colors?.optString(key).orEmpty()) ?: fallback
            // Una URL mal escrita en el panel no debe impedir abrir la app: se ignora ese campo.
            fun url(v: String?): String = if (v.isNullOrBlank()) "" else runCatching { resolve(v) }.getOrDefault("")
            fun text(obj: JSONObject?, key: String): String = obj?.optString(key).orEmpty().trim()

            val stations = buildList {
                val arr = o.optJSONArray("stations")
                for (i in 0 until (arr?.length() ?: 0)) {
                    val s = arr!!.getJSONObject(i)
                    val listen = url(s.optString("listen_url"))
                    if (listen.isEmpty()) continue
                    add(Station(s.optLong("id"), s.optString("label"), s.optString("city"), listen, s.optBoolean("external")))
                }
            }
            val tv = o.optJSONObject("tv")
            val app = o.optJSONObject("app")
            val contact = o.optJSONObject("contact")
            val social = o.optJSONObject("social")
            val links = buildList {
                text(contact, "whatsapp").takeIf { it.isNotEmpty() }?.let {
                    add(LinkItem("WhatsApp", "https://wa.me/" + it.filter(Char::isDigit)))
                }
                text(contact, "phone").takeIf { it.isNotEmpty() }?.let {
                    add(LinkItem("Llamar", "tel:" + it.filter { c -> c.isDigit() || c == '+' }))
                }
                text(contact, "email").takeIf { it.isNotEmpty() }?.let { add(LinkItem("Email", "mailto:$it")) }
                val networks = listOf(
                    "facebook" to "Facebook", "instagram" to "Instagram", "tiktok" to "TikTok",
                    "youtube" to "YouTube", "x" to "X",
                )
                for ((key, label) in networks) {
                    text(social, key).takeIf { it.isNotEmpty() }?.let { add(LinkItem(label, it)) }
                }
            }
            return AppConfig(
                name = o.optString("name").ifBlank { "Club25" },
                slogan = o.optString("slogan"),
                logoUrl = url(o.optString("logo_url")),
                iconUrl = url(o.optString("icon_url")),
                colors = AppColors(
                    bg = color("bg", def.bg), bg2 = color("bg2", def.bg2), card = color("card", def.card),
                    accent = color("accent", def.accent), play = color("play", def.play), text = color("text", def.text),
                ),
                radioEnabled = o.optBoolean("radio_enabled", true),
                stations = stations,
                tvEnabled = tv?.optBoolean("enabled") ?: false,
                tvHlsUrl = url(tv?.optString("hls_url")),
                links = links,
                notice = text(app, "notice"),
                noticeUrl = url(app?.optString("notice_url")),
                minVersion = app?.optInt("min_version") ?: 0,
                playStoreUrl = text(app, "play_store_url"),
                apkUrl = url(app?.optString("apk_url")),
                privacyUrl = url(app?.optString("privacy_url")),
                statusUrl = url(o.optString("status_url")).ifEmpty { resolve("/api/status") },
            )
        }

        private fun parseColor(hex: String): Long? {
            if (!hexColor.matches(hex)) return null
            return 0xFF000000 or hex.substring(1).toLong(16)
        }
    }
}

/** Estado en vivo de una frecuencia. [online] es null si es externa (no se sabe). */
data class StationStatus(
    val online: Boolean?,
    val listeners: Int?,
    val title: String,   // lo que suena
    val program: String, // programa al aire según la programación ("Mañanas Club · DJ")
)

data class LiveStatus(
    val stations: Map<Long, StationStatus>,
    val tvOnline: Boolean,
    val tvViewers: Int,
) {
    companion object {
        fun parse(json: String): LiveStatus {
            val o = JSONObject(json)
            val map = mutableMapOf<Long, StationStatus>()
            val arr = o.optJSONArray("stations")
            for (i in 0 until (arr?.length() ?: 0)) {
                val s = arr!!.getJSONObject(i)
                val onAir = s.optJSONObject("on_air")
                val program = onAir?.let { p ->
                    listOf(p.optString("title"), p.optString("host")).filter { it.isNotBlank() }.joinToString(" · ")
                }.orEmpty()
                map[s.optLong("id")] = StationStatus(
                    online = if (s.isNull("online")) null else s.optBoolean("online"),
                    listeners = if (s.isNull("listeners")) null else s.optInt("listeners"),
                    title = s.optString("title"),
                    program = program,
                )
            }
            val tv = o.optJSONObject("tv")
            return LiveStatus(map, tv?.optBoolean("online") ?: false, tv?.optInt("viewers") ?: 0)
        }
    }
}
