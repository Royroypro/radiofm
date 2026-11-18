package pe.club25.radio.playback

import android.os.SystemClock
import androidx.media3.common.C
import androidx.media3.common.util.UnstableApi
import androidx.media3.exoplayer.audio.TeeAudioProcessor
import java.nio.ByteBuffer
import kotlin.math.PI
import kotlin.math.cos
import kotlin.math.log10
import kotlin.math.sin
import kotlin.math.sqrt

/**
 * Espectro de la radio que suena, para dibujarlo en la pantalla (como el AnalyserNode
 * de la web, site.js §8). Lo escribe [SpectrumAnalyzer] en el hilo de audio y lo lee
 * la interfaz en cada cuadro.
 *
 * El audio se analiza antes de entrar al búfer de salida, así que va por delante de lo
 * que se oye (unos 250 ms de búfer de ExoPlayer más la latencia del teléfono). Para que
 * las barras coincidan con el golpe se guardan los últimos análisis con su hora y la
 * pantalla muestra el de hace [LATENCY_MS] ([delayed]).
 */
object SpectrumTap {
    /** Cantidad de bandas: igual que el analizador de la web (fftSize 256 → 128). */
    const val BANDS = 128

    /** Retraso con el que se muestra el espectro: aprox. lo que tarda el audio en oírse. */
    const val LATENCY_MS = 300L

    private const val KEEP = 64 // ~1,5 s de análisis (uno cada ~23 ms)
    private val frames = arrayOfNulls<FloatArray>(KEEP)
    private val times = LongArray(KEEP)
    private var head = 0 // próxima posición a escribir

    /** Último análisis (sin retraso): valores 0..1 de graves a agudos. */
    @Volatile
    var bands: FloatArray = FloatArray(BANDS)
        private set

    @Volatile
    private var updatedAt = 0L

    /** Hay datos recientes: el audio está pasando por el reproductor. */
    fun live(): Boolean = SystemClock.elapsedRealtime() - updatedAt < 400 + LATENCY_MS

    /** El análisis de hace [LATENCY_MS], es decir, el del audio que se está oyendo ahora. */
    @Synchronized
    fun delayed(): FloatArray {
        val target = SystemClock.elapsedRealtime() - LATENCY_MS
        var best: FloatArray? = null
        var oldest: FloatArray? = null
        for (k in 1..KEEP) { // del más nuevo al más viejo
            val i = (head - k + KEEP) % KEEP
            val f = frames[i] ?: break
            oldest = f
            if (times[i] <= target) {
                best = f
                break
            }
        }
        return best ?: oldest ?: bands
    }

    @Synchronized
    internal fun publish(b: FloatArray) {
        val now = SystemClock.elapsedRealtime()
        frames[head] = b
        times[head] = now
        head = (head + 1) % KEEP
        bands = b
        updatedAt = now
    }

    @Synchronized
    internal fun clear() {
        frames.fill(null)
        times.fill(0L)
        head = 0
        bands = FloatArray(BANDS)
        updatedAt = 0L
    }
}

/**
 * Recibe una copia del audio que va a sonar (lo conecta [PlaybackService] con un
 * TeeAudioProcessor de Media3, que deja pasar el sonido sin cambios) y calcula su
 * espectro con una FFT de 1024 muestras. Lee el audio dentro de la app: no necesita
 * el permiso de micrófono, a diferencia de android.media.audiofx.Visualizer.
 *
 * El análisis va unos cientos de milisegundos por delante de lo que se oye (el
 * búfer de la salida de audio); para un espectro decorativo no se nota.
 */
@UnstableApi
class SpectrumAnalyzer : TeeAudioProcessor.AudioBufferSink {
    private var channels = 2
    private var pcm16 = false
    private val ring = FloatArray(N)
    private var pos = 0
    private var fresh = 0
    private val re = FloatArray(N)
    private val im = FloatArray(N)
    private var smooth = FloatArray(SpectrumTap.BANDS)

    override fun flush(sampleRateHz: Int, channelCount: Int, encoding: @C.PcmEncoding Int) {
        // Solo PCM de 16 bits (lo normal en MP3/AAC); con otro formato no se analiza.
        pcm16 = encoding == C.ENCODING_PCM_16BIT
        channels = channelCount.coerceAtLeast(1)
        fresh = 0
        ring.fill(0f)
        smooth = FloatArray(SpectrumTap.BANDS)
        SpectrumTap.clear()
    }

    override fun handleBuffer(buffer: ByteBuffer) {
        if (!pcm16) return
        val end = buffer.limit()
        val frame = 2 * channels
        var i = buffer.position()
        while (i + frame <= end) { // lecturas absolutas: el búfer no se toca
            var sum = 0
            for (c in 0 until channels) sum += buffer.getShort(i + 2 * c)
            ring[pos] = sum / (channels * 32768f)
            pos = (pos + 1) and (N - 1)
            fresh++
            i += frame
        }
        // Uno por bloque como máximo: al llenar el búfer de salida llegan muchos seguidos.
        if (fresh >= HOP) {
            fresh = 0
            analyze()
        }
    }

    private fun analyze() {
        for (k in 0 until N) {
            re[k] = ring[(pos + k) and (N - 1)] * HANN[k]
            im[k] = 0f
        }
        fft(re, im)
        // 512 frecuencias útiles → 128 bandas (promedio de 4), en dB normalizado como la web
        // (el AnalyserNode mapea de -100 a -30 dB; aquí la escala está ajustada a la FFT).
        val per = (N / 2) / SpectrumTap.BANDS
        val next = FloatArray(SpectrumTap.BANDS)
        for (b in 0 until SpectrumTap.BANDS) {
            var mag = 0f
            for (j in 0 until per) {
                val k = b * per + j
                mag += sqrt(re[k] * re[k] + im[k] * im[k])
            }
            mag = mag / per * (4f / N) // amplitud aproximada 0..1 (Hann pierde la mitad)
            val db = 20f * log10(mag + 1e-9f)
            val v = ((db - MIN_DB) / (MAX_DB - MIN_DB)).coerceIn(0f, 1f)
            smooth[b] = smooth[b] * SMOOTHING + v * (1 - SMOOTHING)
            next[b] = smooth[b]
        }
        SpectrumTap.publish(next)
    }

    private companion object {
        const val N = 1024
        const val HOP = 1024
        const val MIN_DB = -75f
        const val MAX_DB = -12f
        const val SMOOTHING = 0.6f
        val HANN = FloatArray(N) { (0.5 - 0.5 * cos(2 * PI * it / (N - 1))).toFloat() }
        val COS = FloatArray(N / 2) { cos(-2 * PI * it / N).toFloat() }
        val SIN = FloatArray(N / 2) { sin(-2 * PI * it / N).toFloat() }

        /** FFT radix-2 en el lugar (N potencia de 2). */
        fun fft(re: FloatArray, im: FloatArray) {
            val n = re.size
            var j = 0
            for (i in 1 until n) { // reordenamiento por inversión de bits
                var bit = n shr 1
                while (j and bit != 0) {
                    j = j xor bit
                    bit = bit shr 1
                }
                j = j xor bit
                if (i < j) {
                    var t = re[i]; re[i] = re[j]; re[j] = t
                    t = im[i]; im[i] = im[j]; im[j] = t
                }
            }
            var len = 2
            while (len <= n) {
                val step = n / len
                for (i in 0 until n step len) {
                    for (k in 0 until len / 2) {
                        val wr = COS[k * step]
                        val wi = SIN[k * step]
                        val a = i + k
                        val b = a + len / 2
                        val xr = re[b] * wr - im[b] * wi
                        val xi = re[b] * wi + im[b] * wr
                        re[b] = re[a] - xr; im[b] = im[a] - xi
                        re[a] += xr; im[a] += xi
                    }
                }
                len = len shl 1
            }
        }
    }
}
