# Guía del superadmin (proveedor)

El superadmin controla la relación comercial con la radio: planes, contratos, pagos y bloqueos.
El administrador de la radio no ve esta sección. El superadmin puede entrar también a todo el
panel de la radio.

Crear o restablecer un superadmin (solo desde el servidor):
```bash
make superadmin EMAIL=yo@proveedor.pe PASS='clave-segura'
```

## Flujo habitual

1. **Cliente**: razón social, RUC (11 dígitos), contacto.
2. **Planes**: p. ej. "Radio + TV" S/ 350 mensual. Límites (0 = sin límite):
   - máximo de anuncios activos a la vez,
   - tamaño máximo de archivos subidos,
   - incluye TV en vivo (si no, la TV no se muestra ni se acepta la señal de OBS),
   - incluye estadísticas.
3. **Contrato**: plan, fecha de inicio, importe por periodo (se copia del plan), estado
   **Vigente**. El contrato vigente más reciente es el que controla todo.
4. **Pagos**: botón **Registrar pago** en el contrato. Se propone automáticamente el siguiente
   periodo (desde el día siguiente a lo ya pagado, un mes o un año). Método: efectivo,
   transferencia, Yape, Plin, depósito, tarjeta u otro; nº de operación y comprobante.
   - **Pagado**: extiende el "pagado hasta".
   - **Pendiente**: solo informa deuda.
   - **Anulado**: no cuenta.

## Licencia y bloqueos

**Pagado hasta** = fin del periodo pagado más lejano. A partir de ahí:

```
pagado hasta ─► + días de gracia (mora: aviso amarillo) ─► bloqueo automático
```

Niveles de bloqueo:

| Nivel | Efecto |
|---|---|
| Panel en solo lectura | la web sigue igual; el admin de la radio no puede guardar cambios |
| Panel y publicidad | además deja de mostrarse la publicidad |
| Bloqueo total | la web muestra el mensaje de "servicio suspendido"; se cortan la radio y la TV y no pueden reconectar |

- **Bloqueo automático**: elija los días de gracia y el nivel que se aplica al terminar.
- **Bloqueo manual**: aplica un nivel al instante con un motivo, aunque los pagos estén al día.
  Se quita con **Desbloquear**.
- Sin contrato vigente no se aplica ningún control.
- El estado se recalcula cada hora y al instante tras cualquier cambio. También con
  **Recalcular ahora**.

## Auditoría
Lista de todas las acciones (quién, qué, cuándo, IP): inicios de sesión, fallos de login,
cambios en anuncios, ajustes, contratos, pagos, bloqueos y cambios automáticos de nivel
(usuario "sistema").

## Pasarela de pago (futuro)
Hoy los pagos se registran a mano. El código deja la interfaz `billing.PaymentGateway` para
integrar MercadoPago, Culqi, Niubiz, etc. sin cambiar el resto del sistema.
