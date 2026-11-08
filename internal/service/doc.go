// Package service agrupa los casos de uso de la aplicación (capa de negocio),
// cada uno en su subpaquete:
//
//	audit     registro de acciones (quién hizo qué)
//	auth      login, sesiones, usuarios y roles
//	settings  ajustes de marca, contacto y streams (con caché)
//	programs  programación semanal
//	ads       publicidad: CRUD, selección por peso y estadísticas
//	billing   cliente, planes, contratos y pagos (superadmin)
//	license   evaluación de la licencia, bloqueos y límites del plan
//	media     estado en vivo de radio y TV, corte de streams
//	gate      decide si un stream se puede publicar/escuchar (MediaMTX y nginx)
//
// Reglas:
//   - Los servicios reciben repositorios (interfaces de domain) por constructor.
//   - No conocen HTTP: reciben datos ya leídos y devuelven errores de domain.
//   - Validan la entrada y devuelven *domain.ValidationError si algo falla.
//   - Registran en auditoría las acciones que cambian datos.
package service
