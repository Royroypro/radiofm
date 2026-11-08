# ADR 0004 — Usar el nginx del host como proxy HTTPS

- Estado: aceptada
- Fecha: 2026-11-07

## Contexto
El servidor ya tiene nginx en los puertos 80/443 con varios sitios y certificados de certbot.
El plan inicial proponía Caddy en un contenedor.

## Decisión
Un vhost nuevo (`deploy/nginx/radioclub25.sehuacho.com.conf`) con el mismo patrón que el resto,
y certificado con `certbot --nginx`. Los contenedores publican sus puertos HTTP solo en
127.0.0.1. nginx también aplica `auth_request` a los oyentes de la radio.

## Consecuencias
- + No compite por 80/443 con los demás sitios.
- + Cambiar a dominio propio = copiar el vhost y ejecutar certbot.
- − Instalar el vhost requiere sudo (no se automatiza desde Docker).
