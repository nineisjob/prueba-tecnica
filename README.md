# BidCraft

Plataforma de subastas de arte digital de alta concurrencia. Go (API REST + WebSockets) · Astro + React (frontend) · PostgreSQL.

El núcleo de esta prueba técnica no es el CRUD: es demostrar, empíricamente, que **dos pujas en el mismo milisegundo no corrompen el estado de una subasta**, y que el cierre ocurre en el instante exacto sin dejar aceptar pujas tardías ni declarar más de un ganador.

---

## Ejecución rápida

```bash
docker compose up --build
```

- Frontend: http://localhost:4321
- Backend API: http://localhost:8080
- Postgres: `localhost:5433` (usuario/clave `bidcraft`/`bidcraft`)

El backend aplica las migraciones y siembra datos de demo automáticamente al arrancar (usuarios y subastas de ejemplo). Nada más que ejecutar.

**Usuarios de demo** (contraseña `demo1234` para los tres):

| Email | Usuario |
|---|---|
| alice@bidcraft.dev | alice |
| bob@bidcraft.dev | bob |
| carol@bidcraft.dev | carol |

En la vista de detalle de cualquier subasta, el selector de usuario demo permite iniciar sesión con un clic — pensado para grabar el video con dos pestañas compitiendo sin escribir credenciales en cámara.

---

## Sección crítica: cómo se resolvieron las condiciones de carrera

### La garantía en tres capas

| Capa | Mecanismo | Qué evita |
|---|---|---|
| **1. Actor de sala** | Una goroutine por subasta con un único `select` sobre un `chan command` | Que dos pujas se evalúen contra el mismo precio simultáneamente |
| **2. Guarda de expiración** | Verificación explícita de `now >= EndsAt` dentro del manejo de la puja | Que `select` (que elige al azar entre casos listos) entregue una puja encolada por delante de un timer ya disparado |
| **3. UPDATE condicional SQL** | `WHERE status='active' AND now() < ends_at AND $monto >= <regla>` en una transacción | Que la corrección dependa solo del estado en memoria de este proceso (reinicio, segunda réplica, o un bug en las capas 1–2) |

### ¿Por qué Mutex en un sitio y Channels en otro?

El registro de salas (`auction.Manager`) usa `sync.RWMutex`; el estado de cada subasta individual (`auction.Room`) usa un actor alimentado por channels. No es una elección arbitraria:

> El registro de salas es una tabla de lookup **sin semántica de orden** — nadie pregunta "¿se registró la subasta X antes que la Y?". Su acceso está dominado por lecturas: cada puja y cada conexión WebSocket lo consulta; solo la creación y el cierre de una subasta lo escriben. Un `sync.RWMutex` expresa exactamente ese patrón y cuesta unos ~20ns bajo contención de lectura. Modelar el registro como un actor añadiría un salto de goroutine a cada petición y serializaría todos los lookups — convertiría una estructura paralela en lectura en un cuello de botella global, sin ganar ningún invariante nuevo.
>
> **El estado de una puja es lo opuesto:** la decisión de aceptar/rechazar y la decisión de expirar deben observar un **único orden total de eventos**. Eso es precisamente lo que da un `select` alimentado por channels — y lo que un mutex **no puede dar**: un mutex serializa accesos, pero no puede serializar un *timer* contra ellos sin maquinaria adicional (un mutex no tiene forma de "despertar" cuando pasa el tiempo).

Regla aplicada de forma consistente en todo el proyecto (también en `ws.Hub`, que usa el mismo patrón para su mapa de clientes por sala): **channels donde el orden es el invariante; mutexes donde solo importa la exclusión mutua.**

### El código central (`backend/internal/auction/room.go`)

```go
func (r *Room) run(ctx context.Context) {
    defer close(r.done)
    endT := time.NewTimer(time.Until(r.state.EndsAt))
    for {
        select {
        case <-endT.C:            // el timer dispara
            r.finish(ctx)          // persiste 'finished', publica auction.closed
            r.drainRejecting()     // responde a todo lo que quedó encolado
            return
        case c := <-r.cmds:        // llega una puja u otra orden
            r.handleBid(ctx, c)
        case <-ctx.Done():
            return
        }
    }
}
```

`select{}` en Go **elige al azar** entre casos que están listos simultáneamente. Eso significa que un timer ya disparado **no garantiza** que se procese antes que una puja que ya estaba encolada en el buffer del channel. Por eso `handleBid` no confía en el orden de scheduling y vuelve a comprobar el reloj de pared explícitamente:

```go
func (r *Room) handleBid(ctx context.Context, cmd placeBidCmd) {
    now := r.clock.Now()
    if !now.Before(r.state.EndsAt) || r.state.Status == domain.StatusFinished {
        cmd.Reply <- bidOutcome{Err: domain.ErrAuctionEnded}
        return
    }
    // ... valida la regla de negocio, aplica en DB, responde, publica evento
}
```

### El UPDATE condicional (`backend/internal/repository/postgres/auction_repo.go`)

```sql
UPDATE auctions SET
    current_price_cents = $2, current_winner_id = $3,
    bid_count = bid_count + 1, updated_at = now()
WHERE id = $1
  AND status = 'active'
  AND now() < ends_at
  AND $2 >= CASE WHEN bid_count = 0 THEN base_price_cents
                 ELSE current_price_cents + min_increment_cents END
RETURNING ...;
-- RowsAffected == 0  =>  la puja se rechaza
```

**¿Por qué no `SELECT ... FOR UPDATE`?** Mantiene el bloqueo de fila a través de dos viajes de red, y la lógica de aceptar/rechazar termina viviendo en Go *entre* esos dos viajes — la capa SQL deja de ser una verificación independiente y se convierte solo en un candado. Un único UPDATE condicional es atómico en una sola sentencia.

**¿Por qué no `SERIALIZABLE`?** Obliga a un bucle de reintento ante el código de error `40001` en cada llamador — complejidad real para una garantía que el UPDATE condicional ya ofrece sobre una única fila. Descartado por KISS.

**La violación deliberada de DRY:** la regla de puja (`monto >= precio_actual + incremento`) vive tanto en Go (`domain.Auction.MinNextBidCents`) como en el `CASE` de SQL. DRY es la prioridad más baja de las cuatro (SOLID > YAGNI > KISS > DRY); la corrección de grado SOLID es la más alta. Esta duplicación compra corrección ante un reinicio del proceso, una segunda réplica del backend, o cualquier bug en las capas 1–2 — la base de datos actúa como árbitro final independiente. El riesgo de que las dos definiciones diverjan silenciosamente está neutralizado por `TestSQLGuardMatchesDomainRule` (`backend/internal/domain/auction_test.go`), que reimplementa la regla SQL como una función Go y verifica que ambas den siempre el mismo veredicto sobre ~25 casos límite.

### Garantía de cierre exacto

> **Ninguna puja se persiste con un instante de aceptación igual o posterior a `ends_at`, y la transición de cierre ocurre exactamente una vez, porque la ejecuta la misma goroutine que evalúa cada puja.**

Los tres casos posibles en el instante de expiración están todos cubiertos:

1. **Puja encolada, el timer dispara primero, `select` elige la puja de todos modos.** La guarda de la Capa 2 detecta `now >= EndsAt` y responde `ErrAuctionEnded` → HTTP 409. El cierre ocurre en la siguiente iteración del loop.
2. **Puja ya admitida (pasó la guarda), el timer dispara mientras la escritura a la DB está en curso.** El actor está dentro de la llamada a la base de datos y aún no observó el timer. La puja se acepta — **correctamente**, porque fue validada en un instante de reloj estrictamente anterior a `ends_at`. El cierre se ejecuta en cuanto el actor regresa de esa llamada (latencia añadida: ~1ms, un round-trip a la DB).
3. **El reloj de la aplicación dice "antes"; el reloj de Postgres dice "después".** La guarda SQL incluye `AND now() < ends_at`, así que el UPDATE afecta 0 filas → 409. La base de datos es el desempate final, y ninguna fila de puja puede existir para una subasta ya cerrada.

### Réplica única — la asunción explícita

El diseño asume **un único proceso backend**. Con dos réplicas, la Capa 1 (serialización por actor) deja de ser una garantía global — cada réplica serializa *sus propias* pujas, pero dos réplicas podrían, en teoría, aceptar cada una una puja distinta antes de que la Capa 3 (SQL) las reconcilie. La Capa 3 sigue siendo correcta en ese escenario: la segunda réplica que intente aplicar su puja encontrará que el `UPDATE` ya no cumple la condición de precio y la rechazará. **El sistema degrada de "actor serializado" a "concurrencia optimista" — sigue siendo correcto, solo con más rechazos.** Escalar a múltiples réplicas manteniendo la Capa 1 como garantía global requeriría un advisory lock de Postgres por subasta o particionar las subastas por réplica mediante hash consistente — deliberadamente fuera de alcance (YAGNI) para esta prueba de un único proceso.

---

## La prueba empírica

Además de los tests automatizados, `cmd/stress` dispara peticiones HTTP `POST /bids` genuinamente simultáneas (todas las goroutines liberadas por un `close(start)` — una barrera real, no una rampa escalonada) y **verifica contra la API en vivo**, no contra contadores locales.

```bash
npm run stress -- -mode tie -concurrency 500     # el caso más afilado: mismo monto, todas a la vez
npm run stress -- -mode burst -concurrency 500   # cada goroutine puja un monto distinto
```

Resultado real de una corrida en `-mode tie` contra el backend dockerizado:

```
BidCraft stress tool -- base=http://localhost:8080 mode=tie concurrency=500
Authenticated 20 users
Created auction 0158e783-317f-4942-8faf-bdeeda2361b5 (base price 1000 cents)

Accepted: 1  Rejected: 499
Rejection breakdown:
  BID_TOO_LOW              475
  ALREADY_HIGHEST_BIDDER   24
Latency p50=1.44s p95=2.98s p99=3.13s

PASS
```

**500 clientes pujando exactamente el mismo monto al mismo tiempo → exactamente 1 aceptada.** El script verifica además, contra la API: que `count(201) == len(historial de pujas)`, que el historial es estrictamente creciente por `seq`, que `current_price_cents` de la subasta coincide con el máximo monto aceptado, que no hay montos duplicados, y que no ocurrió ningún error 5xx. Sale con código de salida distinto de cero si alguna verificación falla, así que puede usarse como gate de CI.

### Un bug real que esta prueba encontró

Al ejecutar el test de integración `TestApplyBid_ConcurrentDirect` (200 goroutines llamando a `postgres.ApplyBid` directamente, saltándose el actor) contra Postgres real, el proceso se colgaba indefinidamente. La causa: `classifyBidRejection` (la función que da un motivo preciso de rechazo tras un `UPDATE` con 0 filas afectadas) llamaba a `r.GetByID`, que adquiere una **nueva** conexión del pool — mientras la goroutine **todavía sostenía** la conexión de su propia transacción. Bajo carga suficiente para saturar el pool, cada goroutine en esa rama quedaba esperando una segunda conexión que nunca se liberaría, porque todas las demás goroutines estaban en la misma situación: un **auto-deadlock clásico de connection pool**. La corrección fue reutilizar la misma transacción (`tx.QueryRow`) para la lectura de clasificación en vez de abrir una conexión nueva — cero conexiones adicionales, mismo resultado. Ver el comentario en `backend/internal/repository/postgres/auction_repo.go`.

Se dejó también configurable `DB_MAX_CONNS` (por defecto 25) en vez del valor por defecto de pgx, documentando explícitamente que cada subasta activa consume como máximo una conexión a la vez (su actor la serializa), así que el techo real de conexiones lo marca el número de subastas activas simultáneas, no el volumen de pujas.

---

## Arquitectura

### Backend (`backend/`) — Clean Architecture simplificada

```
cmd/api/main.go              wiring: config → pool → migrate → seed → repos → hub → manager → recover → router → server
cmd/stress/main.go           herramienta de ataque concurrente
internal/
  domain/                    entidades, interfaces de repositorio, errores — sin dependencias externas
  auction/                   el motor: room.go (actor), manager.go (registro)
  auth/                      registro/login, depende solo de las interfaces de dominio
  repository/postgres/       implementación de los repositorios; el único paquete que importa pgx
  transport/http/            router (net/http.ServeMux puro, sin framework), middleware, handlers, DTOs
  transport/ws/              hub, cliente, upgrade handler — implementa domain.EventPublisher
  platform/                  config, db, jwt, hash, logx, seed
migrations/                  SQL versionado, embebido en el binario (golang-migrate)
```

**La prueba concreta del desacople OCP/ISP que pide el enunciado:** `internal/auction` no importa `net/http` ni ningún paquete de WebSockets — verificado por un test de arquitectura (`internal/auction/deps_test.go`) que falla el build si esa dependencia aparece algún día. El motor solo conoce `domain.EventPublisher` (una interfaz de una función); ni sabe ni le importa que del otro lado hay un WebSocket.

**Router sin framework:** `net/http.ServeMux` de la librería estándar (Go 1.22+ soporta patrones `"POST /path/{id}"` con wildcards). Cero dependencias de enrutamiento — la señal YAGNI más fuerte posible para un proyecto de este tamaño.

### Frontend (`frontend/`) — Arquitectura de Islas

```
src/
  pages/            index.astro (SSG) · auctions/index.astro (SSR) · auctions/[id].astro (SSR shell + isla)
  components/astro/ Header, Footer, FilterBar (enlaces <a href="?status=">, sin JS), AuctionCard, ...
  components/react/ LiveBiddingRoom.tsx — LA ÚNICA isla client:load de todo el sitio
  hooks/             useWebSocket, useServerTime, useCountdown, useAuctionRoom, useToasts, useAuth
```

Solo la vista de detalle de una subasta hidrata React (`client:load`), y solo el formulario/countdown/feed dentro de ella son interactivos. El catálogo, sus filtros y la landing son HTML puro renderizado en servidor — se puede navegar y filtrar por estado con JavaScript desactivado.

**Detalles de sincronización de reloj (`useServerTime` + `useCountdown`):** el offset cliente-servidor se calcula estilo SNTP (`offset = server_time_ms + (t1-t0)/2 - t1`, mediana de 3 muestras) y el contador **nunca decrementa un valor guardado** — siempre recalcula `ends_at - (Date.now() + offset)` en cada tick, porque una pestaña en segundo plano puede throttlear `setInterval` a más de un segundo y un contador que solo resta se desincronizaría permanentemente. Cuando el contador llega a cero, la UI muestra "Cerrando…" y espera el evento `auction.closed` del servidor — nunca declara un ganador por sí misma; si el WebSocket está caído en ese instante, un fallback hace un `GET` de respaldo a los 3 segundos.

---

## Contrato de API

Ver `backend/internal/transport/http/dto.go` y `errmap.go` para el detalle completo. Resumen:

| Método | Ruta | Auth |
|---|---|---|
| POST | `/api/v1/auth/register`, `/auth/login` | — |
| GET | `/api/v1/auth/me` | JWT |
| GET | `/api/v1/auctions?status=active\|created\|finished\|all` | — |
| POST | `/api/v1/auctions` | JWT |
| GET | `/api/v1/auctions/{id}`, `/{id}/bids` | — |
| POST | `/api/v1/auctions/{id}/bids` | JWT |
| GET | `/api/v1/auctions/{id}/ws` | — (anónimo, solo lectura) |

El WebSocket es intencionalmente anónimo: los navegadores no pueden fijar headers en el constructor `WebSocket`, así que autenticarlo forzaría un `?token=` en la URL (que los proxies loguean). Las pujas van por REST autenticado de todas formas; el feed solo contiene datos públicos.

Cada error sigue un único formato (`transport/http/errmap.go` es la única tabla que mapea errores de dominio a código HTTP):

```json
{"error": {"code": "BID_TOO_LOW", "message": "...", "details": {"min_next_bid_cents": 15000}}}
```

---

## Testing

```bash
npm test                 # unitarios, rápido, sin Docker
npm run test:race        # go test -race dentro de Docker (ver nota de Windows abajo)
npm run test:integration # tests contra Postgres real (db-test)
npm run stress -- -mode tie -concurrency 500
```

- **`internal/domain`**: la regla de puja (`MinNextBidCents`), y `TestSQLGuardMatchesDomainRule` (la tabla de ~25 casos que mantiene sincronizadas las capas Go y SQL).
- **`internal/auction`**: el test central, `TestRoom_HundredsOfSimultaneousBids` (500 goroutines liberadas por una barrera real; verifica que los montos aceptados son estrictamente crecientes, que no hay duplicados, que el ganador final coincide con el monto máximo, y que los eventos publicados están en el mismo orden). Además: `TestRoom_NoBidAcceptedAfterExpiry`, `TestRoom_ExpiryWhileBidInFlight`, `TestManager_BidDuringRoomShutdown` (la carrera lookup/cierre del registro).
- **`internal/repository/postgres`** (tag `integration`): `TestApplyBid_ConcurrentDirect` prueba la Capa 3 completamente sola, sin el actor por delante — la justificación entera de duplicar la regla en SQL.
- **`internal/transport/ws`**: `TestHub_PublishNeverBlocks_SlowClientIsDropped` prueba el contrato de `EventPublisher` (nunca bloquear) desconectando a un cliente lento en vez de frenar el broadcast.

### Nota sobre `-race` en Windows

El detector de carreras de Go necesita cgo, que a su vez necesita un compilador C (MinGW/gcc). Un `winget install GoLang.Go` normal **no** lo trae, así que `go test -race` falla en Windows puro con *"requires cgo"*. Por eso `-race` corre dentro del stage `test` del `Dockerfile` del backend (Alpine + gcc), invocado por `npm run test:race` — funciona igual desde cualquier sistema operativo host.

### Nota sobre el stress tool en Windows nativo

Disparar 500 conexiones TCP genuinamente simultáneas contra `localhost` desde un `go run` nativo de Windows puede toparse con el backlog de conexiones del sistema operativo (`connectex: ... actively refused it`) — un límite del stack TCP de Windows para ráfagas de conexiones *nuevas*, no un bug de la aplicación. Contra el backend dockerizado (Linux) el mismo ataque de 500 conexiones se completa sin ningún error de transporte. El `cmd/stress` incluido ya ajusta el `http.Transport` del cliente Go (`MaxIdleConnsPerHost` elevado) para mitigar esto, pero **la medición de referencia debe tomarse contra el backend en Docker**, como en el ejemplo de salida de arriba.

---

## DevOps

- **`docker-compose.yml`**: `db`, `backend`, `frontend` (los tres que arranca `docker compose up`), más `db-test`/`backend-test` bajo `--profile test` (no arrancan por defecto).
- **Volumen nombrado** para Postgres (nunca un bind mount — falla `initdb` en Windows con permisos).
- **`depends_on: condition: service_healthy`** en vez de la forma corta — sin esto, el backend arrancaría contra un Postgres que todavía no terminó de inicializar y entraría en crash-loop.
- **Healthcheck del backend vía el propio binario** (`bidcraft -healthcheck`): la imagen `runtime` es `distroless` (sin shell, sin curl), así que el healthcheck clásico con `curl` simplemente no puede existir ahí.
- **Sin `platform:` fijado en ningún servicio** → el mismo compose construye igual en Windows/amd64 y Apple Silicon.

### Windows: por qué existe `.gitattributes`

`* text=auto eol=lf` (con overrides para `.ps1`/`.bat`) se commiteó **antes que cualquier otro archivo** del repositorio. Sin eso, un `.sh` editado en Windows entra al historial con `\r\n` y revienta dentro de un contenedor Linux con `bad interpreter: /bin/sh^M`.

---

## Dónde chocan los principios (SOLID > YAGNI > KISS > DRY)

1. **DRY vs. corrección** — la regla de puja vive en Go *y* en SQL. Se resuelve a favor de SOLID (prioridad 1 sobre DRY, prioridad 4); ver "La violación deliberada de DRY" arriba.
2. **DRY vs. el contrato del enunciado** — `bidder.outbid` es derivable de `bid.placed`, pero el enunciado lo pide como evento explícito. Se mantienen ambos: un requisito funcional pesa más que DRY.
3. **SRP vs. KISS** — ¿debería el actor partirse en Validator/Persister/Notifier separados? No: SRP es "una razón para cambiar", no "una operación". La responsabilidad única del actor es *serializar el ciclo de vida de una subasta*; ya delega la regla a `domain.ValidateBid`, la persistencia a `bidStore` y la notificación a `EventPublisher`.
4. **Testabilidad vs. KISS** — `domain.Clock` expone solo `Now()`. Un scheduler de timers falso haría los tests de expiración deterministas, pero cuesta una abstracción entera para evitar timers reales de 100–300ms en los tests, que ya son rápidos y confiables.
5. **YAGNI vs. robustez** — sin reaper periódico, sin rate limiting, sin locking distribuido. La recuperación al arrancar (`Manager.Recover`) más el hecho de que cada sala posee sus propios timers cubre todo caso real de un único proceso; la condición que cambiaría esta respuesta (multi-réplica) está documentada arriba, no omitida en silencio.
6. **YAGNI vs. framework** — sin ORM, sin router externo, sin librería de toasts. Cada dependencia del `go.mod` (`pgx`, `golang-jwt`, `golang-migrate`, `coder/websocket`, `google/uuid`) es load-bearing.
