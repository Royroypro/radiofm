# ADR 0005 — Fuente de Icecast configurable: mezcla interna (bus) o captura

- Estado: aceptada
- Fecha: 2026-11-08

## Contexto
Según la cabina, el micrófono del locutor puede entrar **a una consola física** (y la PC es una
fuente más de la consola) o **directo a la PC** (la PC es el mezclador). Si el mic va a la consola y
la PC enviara a Icecast solo su mezcla interna, los oyentes de la web no oirían al locutor.

## Decisión
- Ajuste "Fuente de Icecast" con dos valores:
  - **`bus`**: la mezcla interna de la PC (música + carts + mic de la PC). Es la misma que va a la
    tarjeta de salida. El ducking automático del mic aplica aquí.
  - **`captura`**: una entrada de línea de la PC que recibe la **salida de la consola**. El codificador
    se alimenta directo del callback de captura (sin mezclar con nada, así no hay deriva de reloj).
    El botón de mic, si se usa, solo baja la música de la PC.
- La salida a la tarjeta (consola/FM) siempre es el bus.

## Consecuencias
- + Sirve para ambas cabinas sin cambiar de programa.
- − En modo `captura` hace falta un cable de la salida de grabación de la consola a la entrada de línea de la PC.
- − La metadata ("lo que suena") sigue saliendo de la PC aunque el audio venga de la consola.
