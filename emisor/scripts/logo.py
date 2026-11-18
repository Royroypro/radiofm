"""Dibuja el logo de Club25 Emisor (el mismo de frontend/dist/logo.svg) y genera:
  build/appicon.png        1024x1024 (Wails lo usa para el .exe)
  build/windows/icon.ico   16..256 px (ícono de Windows)
Uso, desde emisor/:  python scripts/logo.py   (necesita Pillow)
"""
import math
from PIL import Image, ImageDraw

S = 1024 * 4  # se dibuja grande y se reduce: bordes suaves
k = S / 512   # el diseño está en una grilla de 512 como el SVG


def lerp(a, b, t):
    return tuple(int(a[i] + (b[i] - a[i]) * t) for i in range(3))


img = Image.new("RGBA", (S, S), (0, 0, 0, 0))

# Fondo: cuadrado redondeado con degradado diagonal azul.
grad = Image.new("RGBA", (S, S))
gd = ImageDraw.Draw(grad)
top, bottom = (0x1F, 0x3A, 0x6E), (0x0B, 0x12, 0x22)
for i in range(0, 2 * S, 8):
    gd.line([(i, 0), (0, i)], fill=lerp(top, bottom, min(i / (2 * S), 1)) + (255,), width=10)
mask = Image.new("L", (S, S), 0)
ImageDraw.Draw(mask).rounded_rectangle([16 * k, 16 * k, 496 * k, 496 * k], radius=108 * k, fill=255)
img.paste(grad, (0, 0), mask)

d = ImageDraw.Draw(img)
c = 256 * k
# Ondas de transmisión a los dos lados del punto.
for r, alpha in ((105, 255), (175, 190)):
    # PIL dibuja el grosor hacia adentro del recuadro: se agranda medio trazo para centrarlo en r (como el SVG).
    R = (r + 15) * k
    box = [c - R, c - R, c + R, c + R]
    for start in (135, -45):
        d.arc(box, start, start + 90, fill=(255, 255, 255, alpha), width=int(30 * k))
    # extremos redondeados
    for ang in (135, 225, -45, 45):
        x = c + r * k * math.cos(math.radians(ang))
        y = c + r * k * math.sin(math.radians(ang))
        rr = 15 * k
        d.ellipse([x - rr, y - rr, x + rr, y + rr], fill=(255, 255, 255, alpha))

# Punto rojo «al aire».
r = 52 * k
d.ellipse([c - r, c - r, c + r, c + r], fill=(0xD7, 0x26, 0x3D, 255))
hl = 14 * k
d.ellipse([c - 0.4 * r - hl, c - 0.42 * r - hl, c - 0.4 * r + hl, c - 0.42 * r + hl], fill=(0xFF, 0x8A, 0x8A, 110))

out = img.resize((1024, 1024), Image.LANCZOS)
out.save("build/appicon.png")
out.save("build/windows/icon.ico", sizes=[(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)])
print("listo: build/appicon.png, build/windows/icon.ico")
