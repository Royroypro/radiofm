package pe.club25.radio.ui

import android.provider.Settings
import androidx.compose.foundation.Canvas
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.BlendMode
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.CompositingStrategy
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
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
 * y una tapa blanca en el pico.
 * - Con la radio sonando: datos reales del reproductor ([SpectrumTap]).
 * - En reposo: ritmo simulado (128 BPM) a [idle] de amplitud.
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
    val reduced = remember {
        Settings.Global.getFloat(context.contentResolver, Settings.Global.ANIMATOR_DURATION_SCALE, 1f) == 0f
    }
    var frameNanos by remember { mutableLongStateOf(4_000_000_000L) }
    LaunchedEffect(reduced, playing) {
        if (reduced) {
            // Movimiento reducido: un cuadro por segundo solo mientras suena.
            while (playing) {
                frameNanos = System.nanoTime()
                kotlinx.coroutines.delay(1000)
            }
        } else {
            while (true) withFrameNanos { frameNanos = it }
        }
    }
    val state = remember { SpectrumState() }

    Canvas(
        modifier
            .graphicsLayer(alpha = alpha, compositingStrategy = CompositingStrategy.Offscreen)
            // Se desvanece hacia arriba, como la máscara de la web.
            .drawWithContent {
                drawContent()
                drawRect(
                    Brush.verticalGradient(0f to Color.Transparent, 0.45f to Color.Black),
                    blendMode = BlendMode.DstIn,
                )
            },
    ) {
        val t = frameNanos / 1e9
        val w = size.width
        val h = size.height
        val bars = state.resize((w / barWidth.toPx()).roundToInt())
        val half = bars / 2
        val live = playing && SpectrumTap.live()
        val freq = if (live) SpectrumTap.bands else null
        val gap = max(2.dp.toPx(), w / bars * 0.28f)
        val bw = w / bars - gap
        val brush = Brush.verticalGradient(0f to SpectrumSky, 0.45f to orange, 1f to red, startY = 0f, endY = h)
        val cap = 3.dp.toPx()

        for (i in 0 until half) {
            val target = if (freq != null) {
                val bin = floor((i.toFloat() / half).pow(1.6f) * (freq.size * 0.75f)).toInt()
                freq[bin.coerceIn(0, freq.lastIndex)].pow(1.3f)
            } else {
                simulated(i, half, t) * (if (playing) 1f else idle)
            }
            var cur = state.levels[i]
            cur = if (reduced) target else cur + (target - cur) * (if (target > cur) 0.55f else 0.12f)
            state.levels[i] = cur
            state.peaks[i] = max(state.peaks[i] - 0.008f, cur)

            val bh = max(4.dp.toPx(), cur * h * 0.95f)
            val ph = state.peaks[i] * h * 0.95f
            for (k in intArrayOf(half - 1 - i, half + i)) {
                val x = k * (bw + gap) + gap / 2
                drawRect(brush, Offset(x, h - bh), Size(bw, bh))
                drawRect(Color.White.copy(alpha = .9f), Offset(x, h - ph - 2 * cap), Size(bw, cap))
            }
        }
    }
}

/** Niveles y picos de cada barra entre cuadros. */
private class SpectrumState {
    var levels = FloatArray(0)
    var peaks = FloatArray(0)

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
