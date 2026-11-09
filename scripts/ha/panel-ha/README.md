# Panel HA del VPS — cambios de RadioClub25

El panel vive en el VPS (`/opt/ha-panel`, sin git, servicio `ha-panel`, detrás de
HAProxy en `/panel-ha`). `ha-panel-radioclub25.patch` es el cambio aplicado, por si
el panel se reinstala (mismo criterio que `kipu/scripts/ha/ha-panel-kipu.patch`):

- **Menú lateral** con un apartado por sistema desplegado: Infraestructura (VPS,
  recursos por nodo, HAProxy, WireGuard, logs), Restaurante (legado: MySQL,
  failover, acoplamiento, deploy), Kipu y RadioClub25. Cada entrada lleva su
  indicador de estado; el de arriba es el global. Navegación por `#infra`,
  `#legado`, `#kipu`, `#radio`. En móvil el menú pasa a una barra superior.
- **Apartado RadioClub25**: estado de A/B/C, copia, lo que está al aire, tabla de
  puntos (dominio, rutas, ingesta, nodos) y acciones (ver `../README.md`).
- Backend: `rc25_collect` (hilo cada 30 s, SSH a los nodos + `/healthz` del edge),
  `GET /api/radioclub25/status`, `POST /api/radioclub25` (`check`, `sync`,
  `deploy` con `RADIO`, `promover` con `PROMOVER`). Nada del legado ni de Kipu cambia
  de comportamiento: solo se reparten sus tarjetas entre apartados.

Aplicar (desde A; `app.py` e `index.html` de esta carpeta son los ficheros completos):

```bash
scripts/ha/panel-ha/instalar-panel.sh
```

Guarda copias `*.bak-radioclub25-<fecha>` en el VPS, comprueba que `app.py` compila
con el Python del VPS, copia y reinicia `ha-panel`. Si el panel no vuelve a
responder en `127.0.0.1:9099`, restaura las copias y lo reinicia.
