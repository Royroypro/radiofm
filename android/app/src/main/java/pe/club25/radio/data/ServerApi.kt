package pe.club25.radio.data

import android.content.Context
import androidx.core.content.edit
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.net.HttpURLConnection
import java.net.URL

/**
 * Cliente del servidor radiofm. La URL base se fija al compilar (BuildConfig.SERVER_URL).
 * Guarda la última configuración recibida para poder abrir la app sin conexión.
 * No usa librerías HTTP: son dos GET pequeños.
 */
class ServerApi(context: Context, private val baseUrl: String, private val userAgent: String) {
    private val prefs = context.getSharedPreferences("club25", Context.MODE_PRIVATE)

    /** Convierte "/radio" en "https://servidor/radio"; las URL completas no cambian. */
    fun resolve(url: String): String = URL(URL("$baseUrl/"), url).toString()

    /** Última configuración guardada (o null si nunca se recibió). */
    fun cachedConfig(): AppConfig? =
        prefs.getString(KEY_CONFIG, null)?.let { runCatching { AppConfig.parse(it, ::resolve) }.getOrNull() }

    /** Descarga /api/app/config y la guarda. Lanza excepción si no hay conexión o falla. */
    suspend fun fetchConfig(): AppConfig {
        val body = get(resolve("/api/app/config"))
        val cfg = AppConfig.parse(body, ::resolve)
        prefs.edit { putString(KEY_CONFIG, body) }
        return cfg
    }

    /** Descarga el estado en vivo (/api/status). */
    suspend fun fetchStatus(statusUrl: String): LiveStatus = LiveStatus.parse(get(statusUrl))

    private suspend fun get(url: String): String = withContext(Dispatchers.IO) {
        val conn = URL(url).openConnection() as HttpURLConnection
        try {
            conn.connectTimeout = 8_000
            conn.readTimeout = 8_000
            conn.setRequestProperty("Accept", "application/json")
            conn.setRequestProperty("User-Agent", userAgent)
            val code = conn.responseCode
            if (code != HttpURLConnection.HTTP_OK) throw ServerException(code)
            conn.inputStream.bufferedReader().use { it.readText() }
        } finally {
            conn.disconnect()
        }
    }

    /** Respuesta HTTP distinta de 200 (503 = servicio suspendido). */
    class ServerException(val code: Int) : Exception("HTTP $code")

    private companion object {
        const val KEY_CONFIG = "app_config_json"
    }
}
