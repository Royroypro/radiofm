package pe.club25.radio.ui

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import pe.club25.radio.data.AppColors

/**
 * Tema de la app con sus colores propios, que llegan del panel admin («Colores de la app»,
 * aparte de los de la web): fondo con degradado, tarjetas, botón de play, avisos y textos.
 */
@Composable
fun Club25Theme(colors: AppColors?, content: @Composable () -> Unit) {
    val c = colors ?: AppColors()
    val bg = Color(c.bg)
    val card = Color(c.card)
    val text = Color(c.text)
    val scheme = darkColorScheme(
        primary = Color(c.play),
        onPrimary = Color.White,
        secondary = Color(c.accent),
        onSecondary = Color.White,
        tertiary = Color(c.bg2), // abajo del degradado del fondo
        background = bg,
        onBackground = text,
        surface = bg,
        onSurface = text,
        surfaceVariant = card,
        onSurfaceVariant = text,
        surfaceContainer = card,
        surfaceContainerHigh = card,
        primaryContainer = card,
        onPrimaryContainer = text,
    )
    MaterialTheme(colorScheme = scheme, content = content)
}
