package pe.club25.radio.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.asPaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilledIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import coil.compose.AsyncImage
import pe.club25.radio.R
import pe.club25.radio.data.AppConfig
import pe.club25.radio.data.LiveStatus
import pe.club25.radio.data.Station

private val LiveGreen = Color(0xFF2ECC71)
private val PlayIconTint = Color.White // ícono sobre el botón de play

@Composable
fun HomeScreen(
    ui: HomeUi,
    player: PlayerUi,
    appVersion: Int,
    onToggle: (Station) -> Unit,
    onOpenTv: (String) -> Unit,
    onRetry: () -> Unit,
    onOpenUrl: (String) -> Unit,
    onUpdate: () -> Unit,
    onShare: () -> Unit,
) {
    // Fondo con degradado: «Fondo (arriba)» → «Fondo (abajo)» del panel.
    val bg = Brush.verticalGradient(listOf(MaterialTheme.colorScheme.background, MaterialTheme.colorScheme.tertiary))
    Surface(Modifier.fillMaxSize().background(bg), color = Color.Transparent) {
        val cfg = ui.config
        if (cfg == null) {
            NoConfig(ui, onRetry)
            return@Surface
        }
        var updateDismissed by remember { mutableStateOf(false) }
        if (cfg.minVersion > appVersion && !updateDismissed) {
            AlertDialog(
                onDismissRequest = { updateDismissed = true },
                title = { Text("Actualice la app") },
                text = { Text("Hay una versión nueva de ${cfg.name}. Actualícela en Google Play para seguir escuchando sin problemas.") },
                confirmButton = { Button(onClick = onUpdate) { Text("Actualizar") } },
                dismissButton = { TextButton(onClick = { updateDismissed = true }) { Text("Ahora no") } },
            )
        }
        val insets = WindowInsets.safeDrawing.asPaddingValues()
        LazyColumn(
            contentPadding = PaddingValues(
                start = 16.dp, end = 16.dp,
                top = insets.calculateTopPadding() + 16.dp, bottom = insets.calculateBottomPadding() + 24.dp,
            ),
            verticalArrangement = Arrangement.spacedBy(14.dp),
        ) {
            item { Header(cfg, playing = player.playing && !player.error) }
            if (ui.error != null && ui.fromCache) {
                item { Text("Sin conexión: se muestran los últimos datos guardados.", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onBackground.copy(alpha = .7f)) }
            }
            if (cfg.notice.isNotBlank()) {
                item { Notice(cfg, onOpenUrl) }
            }
            if (cfg.radioEnabled && cfg.stations.isNotEmpty()) {
                item { SectionTitle("Radio en vivo") }
                items(cfg.stations, key = { it.id }) { st -> StationCard(st, ui.status, player, onToggle) }
            }
            if (cfg.tvEnabled && cfg.tvHlsUrl.isNotBlank()) {
                item { SectionTitle("TV en vivo") }
                item { TvCard(cfg, ui.status, onOpenTv) }
            }
            if (!cfg.radioEnabled && !cfg.tvEnabled) {
                item { Text("La transmisión no está disponible por ahora.", color = MaterialTheme.colorScheme.onBackground) }
            }
            if (cfg.links.isNotEmpty()) {
                item { SectionTitle("Contacto y redes") }
                item { Links(cfg, onOpenUrl) }
            }
            item { Footer(cfg, onOpenUrl, onShare) }
        }
    }
}

@Composable
private fun NoConfig(ui: HomeUi, onRetry: () -> Unit) {
    Column(
        Modifier.fillMaxSize().padding(32.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        if (ui.loading) {
            CircularProgressIndicator(color = MaterialTheme.colorScheme.secondary)
            Spacer(Modifier.height(16.dp))
            Text("Conectando…", color = MaterialTheme.colorScheme.onBackground)
        } else {
            Text(ui.error ?: "No se pudo cargar la radio.", color = MaterialTheme.colorScheme.onBackground, textAlign = TextAlign.Center)
            Spacer(Modifier.height(16.dp))
            Button(onClick = onRetry) { Text("Reintentar") }
        }
    }
}

@Composable
private fun Header(cfg: AppConfig, playing: Boolean) {
    // Detrás del logo, el espectro de audio como en la portada de la web.
    Box(Modifier.fillMaxWidth()) {
        AudioSpectrum(
            playing = playing,
            red = MaterialTheme.colorScheme.primary,
            orange = MaterialTheme.colorScheme.secondary,
            modifier = Modifier.matchParentSize(),
        )
        HeaderContent(cfg)
    }
}

@Composable
private fun HeaderContent(cfg: AppConfig) {
    Column(Modifier.fillMaxWidth().padding(top = 8.dp, bottom = 20.dp), horizontalAlignment = Alignment.CenterHorizontally) {
        if (cfg.logoUrl.isNotBlank()) {
            AsyncImage(
                model = cfg.logoUrl,
                contentDescription = cfg.name,
                modifier = Modifier.height(96.dp).fillMaxWidth(),
            )
        } else {
            Text(cfg.name, style = MaterialTheme.typography.headlineMedium, fontWeight = FontWeight.Black, color = MaterialTheme.colorScheme.onBackground)
        }
        if (cfg.slogan.isNotBlank()) {
            Spacer(Modifier.height(6.dp))
            Text(cfg.slogan, style = MaterialTheme.typography.titleMedium, color = MaterialTheme.colorScheme.onBackground.copy(alpha = .85f), textAlign = TextAlign.Center)
        }
    }
}

@Composable
private fun Notice(cfg: AppConfig, onOpenUrl: (String) -> Unit) {
    val clickable = cfg.noticeUrl.isNotBlank()
    Surface(
        color = MaterialTheme.colorScheme.secondary,
        shape = RoundedCornerShape(14.dp),
        modifier = Modifier.fillMaxWidth().then(if (clickable) Modifier.clickable { onOpenUrl(cfg.noticeUrl) } else Modifier),
    ) {
        Text(
            cfg.notice + if (clickable) "  ›" else "",
            modifier = Modifier.padding(14.dp),
            color = MaterialTheme.colorScheme.onBackground, fontWeight = FontWeight.SemiBold,
        )
    }
}

@Composable
private fun SectionTitle(text: String) {
    Text(text, style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.onBackground,
        modifier = Modifier.padding(top = 6.dp))
}

/** Etiqueta «En vivo» / «Fuera del aire» / «Señal externa». */
@Composable
private fun LiveBadge(online: Boolean?) {
    val (text, color) = when (online) {
        true -> "EN VIVO" to LiveGreen
        false -> "FUERA DEL AIRE" to MaterialTheme.colorScheme.onBackground.copy(alpha = .55f)
        null -> "SEÑAL EXTERNA" to MaterialTheme.colorScheme.secondary
    }
    Row(verticalAlignment = Alignment.CenterVertically) {
        Box(Modifier.size(9.dp).clip(CircleShape).background(color))
        Spacer(Modifier.width(6.dp))
        Text(text, style = MaterialTheme.typography.labelMedium, fontWeight = FontWeight.Bold, color = color)
    }
}

@Composable
private fun StationCard(st: Station, status: LiveStatus?, player: PlayerUi, onToggle: (Station) -> Unit) {
    val s = status?.stations?.get(st.id)
    val online: Boolean? = if (st.external) null else (s?.online ?: false)
    val current = player.stationId == st.id
    val playing = current && player.playing && !player.error
    Card(
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceVariant),
        shape = RoundedCornerShape(18.dp),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Box {
            // Mientras suena esta frecuencia, su espectro al fondo de la tarjeta.
            if (playing) {
                AudioSpectrum(
                    playing = true,
                    red = MaterialTheme.colorScheme.primary,
                    orange = MaterialTheme.colorScheme.secondary,
                    modifier = Modifier.matchParentSize(),
                    barWidth = 16.dp,
                    alpha = 0.45f,
                )
            }
            Row(Modifier.padding(16.dp), verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    LiveBadge(online)
                    Spacer(Modifier.height(4.dp))
                    Text(st.label, style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Black, color = MaterialTheme.colorScheme.onBackground)
                    if (st.city.isNotBlank()) Text(st.city, color = MaterialTheme.colorScheme.onBackground.copy(alpha = .8f))
                    val line = when {
                        s?.title?.isNotBlank() == true -> "♪ " + s.title
                        s?.program?.isNotBlank() == true -> s.program
                        else -> ""
                    }
                    if (line.isNotBlank()) {
                        Spacer(Modifier.height(4.dp))
                        Text(line, maxLines = 2, overflow = TextOverflow.Ellipsis, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onBackground)
                    }
                    s?.listeners?.takeIf { it > 0 }?.let {
                        Text(if (it == 1) "1 oyente" else "$it oyentes", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onBackground.copy(alpha = .7f))
                    }
                    if (current && player.error) {
                        Text("No se pudo reproducir. Toque para reintentar.", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.secondary)
                    }
                }
                Spacer(Modifier.width(12.dp))
                Box(contentAlignment = Alignment.Center) {
                    FilledIconButton(
                        onClick = { onToggle(st) },
                        modifier = Modifier.size(64.dp).semantics { contentDescription = (if (playing) "Pausar " else "Escuchar ") + st.label },
                        colors = IconButtonDefaults.filledIconButtonColors(containerColor = MaterialTheme.colorScheme.primary),
                    ) {
                        Icon(painterResource(if (playing) R.drawable.ic_pause else R.drawable.ic_play), contentDescription = null,
                            modifier = Modifier.size(34.dp), tint = PlayIconTint)
                    }
                    if (current && player.buffering) {
                        CircularProgressIndicator(Modifier.size(72.dp), color = PlayIconTint, strokeWidth = 3.dp)
                    }
                }
            }
        }
    }
}

@Composable
private fun TvCard(cfg: AppConfig, status: LiveStatus?, onOpenTv: (String) -> Unit) {
    val online = status?.tvOnline ?: false
    Card(
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceVariant),
        shape = RoundedCornerShape(18.dp),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(Modifier.padding(16.dp)) {
            LiveBadge(online)
            Spacer(Modifier.height(4.dp))
            Text("${cfg.name} TV", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Black, color = MaterialTheme.colorScheme.onBackground)
            Text(
                if (online) "Mírenos en vivo a pantalla completa." else "La TV está fuera del aire. Vuelva más tarde.",
                color = MaterialTheme.colorScheme.onBackground.copy(alpha = .85f),
            )
            if (online && (status?.tvViewers ?: 0) > 0) {
                Text("${status!!.tvViewers} viendo ahora", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onBackground.copy(alpha = .7f))
            }
            Spacer(Modifier.height(12.dp))
            Button(
                onClick = { onOpenTv(cfg.tvHlsUrl) },
                enabled = online,
                colors = ButtonDefaults.buttonColors(containerColor = MaterialTheme.colorScheme.primary),
                modifier = Modifier.fillMaxWidth(),
            ) {
                Icon(painterResource(R.drawable.ic_tv), contentDescription = null, tint = PlayIconTint)
                Spacer(Modifier.width(8.dp))
                Text("Ver TV en vivo")
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun Links(cfg: AppConfig, onOpenUrl: (String) -> Unit) {
    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        cfg.links.forEach { l ->
            OutlinedButton(onClick = { onOpenUrl(l.url) }) { Text(l.label, color = MaterialTheme.colorScheme.onBackground) }
        }
    }
}

@Composable
private fun Footer(cfg: AppConfig, onOpenUrl: (String) -> Unit, onShare: () -> Unit) {
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.Center, verticalAlignment = Alignment.CenterVertically) {
        TextButton(onClick = onShare) { Text("Compartir app", color = MaterialTheme.colorScheme.onBackground.copy(alpha = .8f)) }
        if (cfg.privacyUrl.isNotBlank()) {
            TextButton(onClick = { onOpenUrl(cfg.privacyUrl) }) { Text("Privacidad", color = MaterialTheme.colorScheme.onBackground.copy(alpha = .8f)) }
        }
    }
}
