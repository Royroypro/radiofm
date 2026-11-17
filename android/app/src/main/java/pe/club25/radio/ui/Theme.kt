package pe.club25.radio.ui

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import pe.club25.radio.data.BrandColors

/**
 * Tema de la app con los colores de la marca que llegan del panel admin:
 * azul marino de fondo, azul rey en tarjetas, rojo de acento y naranja secundario.
 */
@Composable
fun Club25Theme(colors: BrandColors?, content: @Composable () -> Unit) {
    val c = colors ?: BrandColors()
    val navy = Color(c.navy)
    val royal = Color(c.royal)
    val scheme = darkColorScheme(
        primary = Color(c.red),
        onPrimary = Color.White,
        secondary = Color(c.orange),
        onSecondary = Color.White,
        background = navy,
        onBackground = Color.White,
        surface = navy,
        onSurface = Color.White,
        surfaceVariant = royal,
        onSurfaceVariant = Color.White,
        surfaceContainer = royal,
        surfaceContainerHigh = royal,
        primaryContainer = royal,
        onPrimaryContainer = Color.White,
    )
    MaterialTheme(colorScheme = scheme, content = content)
}
