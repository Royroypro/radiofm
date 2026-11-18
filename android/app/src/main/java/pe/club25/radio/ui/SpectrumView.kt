package pe.club25.radio.ui

import android.app.ActivityManager
import android.content.Context
import android.provider.Settings
import androidx.compose.foundation.layout.Spacer
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawWithCache
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import pe.club25.radio.playback.SpectrumTap
import kotlin.math.exp
import kotlin.math.floor
import kotlin.math.max
import kotlin.math.min
import kotlin.math.pow
import kotlin.math.roundToInt
import kotlin.math.sin

private val SpectrumSky = Color(0xFF6F87FF) // --sky de la web

/**
 * Espectro de audio como el de la web (site.js §8): barras simétricas desde el
 * centro (graves al medio, agudos a los lados), degradado rojo → naranja → celeste
 * que se desvanece hacia arriba, y una tapa blanca en el pico.
 * - Con la radio sonando: datos reales del reproductor ([SpectrumTap]).
 * - En reposo: ritmo simulado (128 BPM) a [idle] de amplitud.
 *
 * Pensado para teléfonos modestos:
 * - Cuadros limitados: 30 por segundo sonando y 15 en reposo (en teléfonos con poca
 *   memoria, 24 y 10, y barras más anchas), no los 60–120 de la pantalla.
 * - Sin capa fuera de pantalla: el desvanecido va en el propio degradado y en la
 *   transparencia de las tapas.
 * - Degradado y arreglos se crean solo al cambiar el tamaño, no en cada cuadro.
 * - El movimiento depende del tiempo, no de los cuadros: se ve igual a cualquier ritmo.
 * Si el usuario quitó las animaciones del sistema se dibuja quieto.
 */
@Composable
fun AudioSpectrum(
    playing: Boolean,
    red: Color,
    orange: Color,
    modifier: Modifier = Modifier,
    barWidth: Dp = 22.dp,
    idle: Float = 0.75f,
    alpha: Float = 0.6f,
) {
    val context = LocalContext.current
    val device = remember { SpectrumDevice.of(context) }
    var frameNanos by remember { mutableLongStateOf(4_000_000_000L) }
    LaunchedEffect(device, playing) {
        if (device.reduced) {
            // Movimiento reducido: un cuadro por segundo solo mientras suena.
            while (playing) {
                frameNanos = System.nanoTime()
                delay(1000)
            }
            return@LaunchedEffect
        }
        val every = 1_000_000_000L / (if (playing) device.fpsPlaying else device.fpsIdle)
        var last = 0L
        while (true) {
            withFrameNanos { t ->
                if (t - last >= every - 2_000_000L) { // margen para no perder cuadros por poco
                    last = t
                    frameNanos = t
                }
            }
        }
    }
    val state = remember { SpectrumState() }

    Spacer(
        modifier.drawWithCache {
            // Todo lo que depende del tamaño se calcula una vez (o al cambiar el tamaño).
            val w = size.width
            val h = size.height
            val bars = state.resize((w / (barWidth.toPx() * device.barScale)).roundToInt())
            val half = bars / 2
            val gap = max(2.dp.toPx(), w / bars * 0.28f)
            val bw = w / bars - gap
            val cap = 3.dp.toPx()
            val minBar = 4.dp.toPx()
            val fadeEnd = h * 0.45f // como la máscara de la web: transparente arriba
            val brush = Brush.verticalGradient(
                0f to SpectrumSky.copy(alpha = 0f),
                0.45f to orange.copy(alpha = alpha),
                1f to red.copy(alpha = alpha),
                startY = 0f, endY = h,
            )
            val xs = FloatArray(bars) { k -> k * (bw + gap) + gap / 2 }
            val barSize = Size(bw, 0f)

            onDrawBehind {
                val now = frameNanos // leer el estado aquí solo redibuja, no recompone
                val dt = state.step(now)
                val t = now / 1e9
                val live = playing && SpectrumTap.live()
                val freq = if (live) SpectrumTap.delayed() else null // el que se está oyendo
                // Suavizado por tiempo (equivale al de la web a 60 cuadros por segundo).
                val up = 1f - (1f - 0.55f).pow(dt * 60f)
                val down = 1f - (1f - 0.12f).pow(dt * 60f)
                val peakFall = 0.008f * dt * 60f

                for (i in 0 until half) {
                    val target = if (freq != null) {
                        val bin = floor((i.toFloat() / half).pow(1.6f) * (freq.size * 0.75f)).toInt()
                        freq[bin.coerceIn(0, freq.lastIndex)].pow(1.3f)
                    } else {
                        simulated(i, half, t) * (if (playing) 1f else idle)
                    }
                    var cur = state.levels[i]
                    cur = if (device.reduced) target else cur + (target - cur) * (if (target > cur) up else down)
                    state.levels[i] = cur
                    val peak = max(state.peaks[i] - peakFall, cur)
                    state.peaks[i] = peak

                    val bh = max(minBar, cur * h * 0.95f)
                    val py = h - peak * h * 0.95f - 2 * cap
                    // Tapa: más transparente cuanto más arriba (mismo desvanecido que las barras).
                    val capAlpha = alpha * 0.9f * (py / fadeEnd).coerceIn(0f, 1f)
                    val left = xs[half - 1 - i]
                    val right = xs[half + i]
                    drawRect(brush, Offset(left, h - bh), barSize.copy(height = bh))
                    drawRect(brush, Offset(right, h - bh), barSize.copy(height = bh))
                    if (capAlpha > 0.02f) {
                        drawRect(Color.White, Offset(left, py), Size(bw, cap), alpha = capAlpha)
                        drawRect(Color.White, Offset(right, py), Size(bw, cap), alpha = capAlpha)
                    }
                }
            }
        },
    )
}

/** Cuánto puede animar este teléfono. */
private data class SpectrumDevice(
    val reduced: Boolean,  // animaciones del sistema desactivadas
    val fpsPlaying: Int,
    val fpsIdle: Int,
    val barScale: Float,   // >1 = barras más anchas (menos barras que dibujar)
) {
    companion object {
        fun of(context: Context): SpectrumDevice {
            val reduced = Settings.Global.getFloat(context.contentResolver, Settings.Global.ANIMATOR_DURATION_SCALE, 1f) == 0f
            val am = context.getSystemService(Context.ACTIVITY_SERVICE) as? ActivityManager
            val low = am?.isLowRamDevice == true || Runtime.getRuntime().availableProcessors() <= 4
            return if (low) SpectrumDevice(reduced, 24, 10, 1.5f) else SpectrumDevice(reduced, 30, 15, 1f)
        }
    }
}

/** Niveles y picos de cada barra entre cuadros, y el tiempo del cuadro anterior. */
private class SpectrumState {
    var levels = FloatArray(0)
    var peaks = FloatArray(0)
    private var lastNanos = 0L

    /** Ajusta la cantidad de barras (par, entre 16 y 72) y devuelve el total. */
    fun resize(wanted: Int): Int {
        var bars = wanted.coerceIn(16, 72)
        if (bars % 2 != 0) bars += 1
        if (levels.size != bars / 2) {
            levels = FloatArray(bars / 2)
            peaks = FloatArray(bars / 2)
        }
        return bars
    }

    /** Segundos desde el cuadro anterior (acotado, por si la app estuvo en pausa). */
    fun step(now: Long): Float {
        val dt = if (lastNanos == 0L) 1f / 30 else ((now - lastNanos) / 1e9f).coerceIn(0f, 0.1f)
        lastNanos = now
        return dt
    }
}

/** Valor 0..1 de la barra i en reposo: bombo a 128 BPM, contratiempo y vaivén (igual que la web). */
private fun simulated(i: Int, half: Int, t: Double): Float {
    val x = i.toDouble() / half
    val beat = (t * 128 / 60) % 1
    val kick = exp(-beat * 7)
    val off = exp(-((beat + 0.5) % 1) * 9) * 0.45
    val wobble = 0.5 + 0.5 * sin(t * 2.3 + i * 0.55) * sin(t * 1.1 + i * 0.21)
    val bass = (1 - x) * (0.35 + 0.65 * kick)
    val highs = x * (0.25 + off) * (0.6 + 0.4 * sin(t * 9 + i))
    return min(1.0, 0.12 + bass * 0.85 + highs * 0.7 + wobble * 0.25).toFloat()
}
