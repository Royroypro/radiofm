package pe.club25.radio.ui

import android.app.Application
import android.util.Log
import androidx.lifecycle.AndroidViewModel
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import pe.club25.radio.BuildConfig
import pe.club25.radio.data.AppConfig
import pe.club25.radio.data.LiveStatus
import pe.club25.radio.data.ServerApi

/** Lo que muestra la pantalla principal. */
data class HomeUi(
    val config: AppConfig? = null,
    val status: LiveStatus? = null,
    val loading: Boolean = true,
    val error: String? = null,    // último error al pedir la configuración
    val fromCache: Boolean = false, // la configuración es la guardada (sin conexión)
)

/**
 * Pide la configuración (/api/app/config) cada vez que la app vuelve a primer
 * plano, así los cambios del panel admin se ven al volver a abrirla, y consulta
 * el estado en vivo (/api/status) cada [STATUS_EVERY_MS] mientras está visible.
 */
class HomeViewModel(app: Application) : AndroidViewModel(app) {
    val api = ServerApi(app, BuildConfig.SERVER_URL, "Club25Android/${BuildConfig.VERSION_NAME}")

    private val _ui = MutableStateFlow(HomeUi(config = api.cachedConfig()).let { it.copy(fromCache = it.config != null) })
    val ui: StateFlow<HomeUi> = _ui

    suspend fun refreshConfig() {
        _ui.update { it.copy(loading = true) }
        try {
            val cfg = api.fetchConfig()
            _ui.update { it.copy(config = cfg, loading = false, error = null, fromCache = false) }
        } catch (e: ServerApi.ServerException) {
            val msg = if (e.code == 503) "El servicio no está disponible en este momento." else "El servidor respondió con un error (${e.code})."
            _ui.update { it.copy(loading = false, error = msg) }
        } catch (e: Exception) {
            Log.w(TAG, "no se pudo leer la configuración", e)
            _ui.update { it.copy(loading = false, error = "No se pudo conectar. Revise su conexión a Internet.") }
        }
    }

    /** Consulta el estado en vivo hasta que se cancela la corrutina (al pasar a segundo plano). */
    suspend fun pollStatus() {
        while (currentCoroutineContext().isActive) {
            val url = _ui.value.config?.statusUrl ?: api.resolve("/api/status")
            runCatching { api.fetchStatus(url) }.onSuccess { st -> _ui.update { it.copy(status = st) } }
            delay(STATUS_EVERY_MS)
        }
    }

    private companion object {
        const val STATUS_EVERY_MS = 15_000L
        const val TAG = "Club25"
    }
}
