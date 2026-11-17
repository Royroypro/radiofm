package pe.club25.radio.ui

import android.content.ActivityNotFoundException
import android.content.ComponentName
import android.content.Intent
import androidx.core.net.toUri
import android.os.Bundle
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.viewModels
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.core.content.ContextCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.repeatOnLifecycle
import androidx.media3.common.MediaItem
import androidx.media3.common.MediaMetadata
import androidx.media3.common.Player
import androidx.media3.session.MediaController
import androidx.media3.session.SessionToken
import com.google.common.util.concurrent.ListenableFuture
import kotlinx.coroutines.launch
import pe.club25.radio.BuildConfig
import pe.club25.radio.data.AppConfig
import pe.club25.radio.data.Station
import pe.club25.radio.playback.PlaybackService

/** Estado del reproductor de radio (servicio en segundo plano) para la pantalla. */
data class PlayerUi(
    val stationId: Long? = null, // frecuencia cargada en el reproductor
    val playing: Boolean = false, // reproduciendo o por reproducir (playWhenReady)
    val buffering: Boolean = false,
    val error: Boolean = false,
)

/**
 * Pantalla principal: reproductores de cada frecuencia, TV en vivo y contacto.
 * El audio lo reproduce [PlaybackService]; aquí solo se controla con un MediaController.
 */
class MainActivity : ComponentActivity() {
    private val vm: HomeViewModel by viewModels()
    private var controllerFuture: ListenableFuture<MediaController>? = null
    private var controller: MediaController? = null
    private var playerUi by mutableStateOf(PlayerUi())

    private val listener = object : Player.Listener {
        override fun onEvents(player: Player, events: Player.Events) = updatePlayerUi(player)
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            val ui by vm.ui.collectAsStateWithLifecycle()
            Club25Theme(ui.config?.colors) {
                HomeScreen(
                    ui = ui,
                    player = playerUi,
                    appVersion = BuildConfig.VERSION_CODE,
                    onToggle = ::toggle,
                    onOpenTv = { url -> startActivity(TvActivity.intent(this, url)) },
                    onRetry = { lifecycleScope.launch { vm.refreshConfig() } },
                    onOpenUrl = ::openUrl,
                    onUpdate = ::openStore,
                    onShare = ::share,
                )
            }
        }
        // Al volver a primer plano: configuración nueva (cambios del panel) y estado en vivo.
        lifecycleScope.launch {
            repeatOnLifecycle(Lifecycle.State.STARTED) {
                vm.refreshConfig()
                vm.pollStatus()
            }
        }
    }

    override fun onStart() {
        super.onStart()
        val token = SessionToken(this, ComponentName(this, PlaybackService::class.java))
        val future = MediaController.Builder(this, token).buildAsync()
        controllerFuture = future
        future.addListener({
            val c = runCatching { future.get() }.getOrNull() ?: return@addListener
            controller = c
            c.addListener(listener)
            updatePlayerUi(c)
        }, ContextCompat.getMainExecutor(this))
    }

    override fun onStop() {
        controller?.removeListener(listener)
        controller = null
        controllerFuture?.let { MediaController.releaseFuture(it) }
        controllerFuture = null
        super.onStop()
    }

    private fun updatePlayerUi(p: Player) {
        playerUi = PlayerUi(
            stationId = p.currentMediaItem?.mediaId?.toLongOrNull(),
            playing = p.playWhenReady && p.playbackState != Player.STATE_IDLE,
            buffering = p.playWhenReady && p.playbackState == Player.STATE_BUFFERING,
            error = p.playerError != null,
        )
    }

    /** Reproduce o pausa una frecuencia. Al reanudar se vuelve a cargar para ir al directo. */
    private fun toggle(station: Station) {
        val c = controller ?: return
        val cfg: AppConfig = vm.ui.value.config ?: return
        if (c.currentMediaItem?.mediaId == station.id.toString() && c.playWhenReady && c.playerError == null) {
            c.pause()
            return
        }
        val meta = MediaMetadata.Builder()
            .setTitle("${cfg.name} ${station.label}")
            .setArtist(station.city.ifBlank { cfg.slogan })
            .setStation(station.label)
            .setIsPlayable(true)
            .setIsBrowsable(false)
        // El sistema no dibuja SVG en la notificación: solo se usa el isotipo si es PNG/JPG/WebP.
        if (cfg.iconUrl.isNotBlank() && !cfg.iconUrl.endsWith(".svg", ignoreCase = true)) {
            meta.setArtworkUri(cfg.iconUrl.toUri())
        }
        c.setMediaItem(
            MediaItem.Builder()
                .setMediaId(station.id.toString())
                .setUri(station.listenUrl)
                .setMediaMetadata(meta.build())
                .build(),
        )
        c.prepare()
        c.play()
    }

    private fun openUrl(url: String) {
        try {
            startActivity(Intent(Intent.ACTION_VIEW, url.toUri()))
        } catch (_: ActivityNotFoundException) {
            Toast.makeText(this, "No hay una app para abrir este enlace", Toast.LENGTH_SHORT).show()
        }
    }

    /** Abre la ficha en Google Play (la del panel o la de este paquete). */
    private fun openStore() {
        val url = vm.ui.value.config?.playStoreUrl?.takeIf { it.isNotBlank() }
            ?: "https://play.google.com/store/apps/details?id=$packageName"
        openUrl(url)
    }

    private fun share() {
        val cfg = vm.ui.value.config
        val link = cfg?.playStoreUrl?.takeIf { it.isNotBlank() } ?: BuildConfig.SERVER_URL
        val text = "Escucha ${cfg?.name ?: "Club25"} en vivo: $link"
        startActivity(Intent.createChooser(Intent(Intent.ACTION_SEND).setType("text/plain").putExtra(Intent.EXTRA_TEXT, text), "Compartir"))
    }
}
