# Identidad visual (provisional)

Basada en las piezas de `imagenes_referecia/`:

| Archivo | Contenido |
|---|---|
| `logo-fondo-navy.jpeg` | logotipo "RADIO CLUB 25 · 93.3 FM" sobre azul marino |
| `arte-circulos-1.jpeg`, `arte-circulos-2.jpeg` | logo + eslogan "La música que va contigo" sobre círculos |
| `arte-categorias-1.jpeg`, `arte-categorias-2.jpeg` | isotipo "25" y categorías Música / Noticias / Éxitos / Retro / Entretenimiento |

## Marca
- Nombre: **Radio Club 25** · Frecuencia: **93.3 FM**
- Eslogan: **La música que va contigo**
- Categorías: Música, Noticias, Éxitos, Retro, Entretenimiento

## Paleta

| Variable CSS | Color | Uso |
|---|---|---|
| `--navy` | `#0A1A6B` | fondo principal |
| `--navy-deep` | derivado (más oscuro) | cabecera, pie, reproductor fijo |
| `--royal` | `#223CB8` | disco central, tarjetas |
| `--red` | `#FF1E1E` | acento: botón play, "en vivo", programa al aire |
| `--orange` | `#FF961E` | acento secundario: títulos pequeños, anillos |
| `--sky` | `#6F87FF` | texto secundario sobre azul (categorías) |
| blanco | `#FFFFFF` | texto y anillos |

Los cuatro primeros se editan en **Admin → Marca y contacto → Colores**.

## Tipografía (Google Fonts)
- Títulos y logotipo: **Playfair Display** Black Italic (similar al "CLUB 25" del logo).
- Texto, etiquetas y eslogan: **Montserrat** (400–800), etiquetas en mayúsculas con espaciado amplio.

## Estilo
Círculos y anillos superpuestos con sombra suave, puntos de color sueltos, alto contraste,
bordes redondeados. Implementado solo con CSS (`.circles` en `site.css`), sin imágenes.

## Logo provisional
- `web/static/img/brand/logo.svg`: "RADIO / CLUB / 93.3 FM" + púa roja con órbita y "25".
- `web/static/img/brand/isotipo.svg`: "25" en círculo (favicon y móvil).

Son recreaciones aproximadas para empezar. Cuando la radio entregue el logo oficial (PNG
transparente o SVG), súbalo en **Admin → Marca y contacto → Imágenes**.
