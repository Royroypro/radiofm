package pe.club25.radio.playback

import androidx.media3.common.C
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.nio.ByteBuffer
import java.nio.ByteOrder
import kotlin.math.PI
import kotlin.math.sin

/** El espectro de un tono puro debe tener su máximo en la banda de esa frecuencia. */
class SpectrumTest {
    @Test
    fun toneLandsInItsBand() {
        val rate = 44_100
        val hz = 1_000.0
        val frames = 4096
        val buf = ByteBuffer.allocate(frames * 4).order(ByteOrder.nativeOrder())
        for (i in 0 until frames) {
            val v = (sin(2 * PI * hz * i / rate) * 16_000).toInt().toShort()
            buf.putShort(v).putShort(v) // estéreo
        }
        buf.flip()

        val a = SpectrumAnalyzer()
        a.flush(rate, 2, C.ENCODING_PCM_16BIT)
        repeat(6) { a.handleBuffer(buf) } // varios bloques: el suavizado llega a su valor
        assertEquals("no debe mover el búfer", 0, buf.position())

        val bands = SpectrumTap.bands
        val peak = bands.indices.maxBy { bands[it] }
        val bandHz = rate / 2.0 / SpectrumTap.BANDS // ≈172 Hz por banda
        assertEquals((hz / bandHz).toInt(), peak)
        assertTrue("el tono debe verse alto", bands[peak] > 0.5f)
        assertTrue("lejos del tono, bajo", bands[SpectrumTap.BANDS - 1] < 0.2f)
    }
}
