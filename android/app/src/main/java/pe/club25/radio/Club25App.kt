package pe.club25.radio

import android.app.Application
import coil.ImageLoader
import coil.ImageLoaderFactory
import coil.decode.SvgDecoder

/**
 * Aplicación: configura Coil para que cargue también logos SVG
 * (el logo por defecto de la web es un SVG).
 */
class Club25App : Application(), ImageLoaderFactory {
    override fun newImageLoader(): ImageLoader =
        ImageLoader.Builder(this)
            .components { add(SvgDecoder.Factory()) }
            .crossfade(true)
            .build()
}
