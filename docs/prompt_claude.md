# Prompt de Instrucción General para Claude Code: BidCraft - Plataforma de Subastas

**Objetivo de la Sesión:** 
Asume el rol de un Arquitecto DevOps y Senior Full Stack Engineer. Tu tarea es diseñar, implementar y documentar la prueba técnica "BidCraft", una plataforma de subastas de arte digital de alta concurrencia. Debes generar un código listo para producción, enfocado en el rendimiento en tiempo real y la prevención de condiciones de carrera.

---

## 1. Reglas Generales y Principios de Arquitectura (ORDEN DE PRIORIDAD ESTRICTO)

Toda decisión arquitectónica, patrón de diseño y línea de código escrita debe someterse a los siguientes principios, evaluados **exactamente en este orden de prioridad**:

1. **SOLID (Prioridad Máxima):** 
   - Aplica el Principio de Responsabilidad Única (SRP) en los handlers de Go y los componentes de Astro. Aisla la lógica de negocio de la lógica de entrega (HTTP/WS).
   - Utiliza Inyección de Dependencias (DIP) en el backend (ej. inyectar interfaces de repositorios para la base de datos).
   - Asegura que el motor de concurrencia y la conexión a WebSockets estén desacoplados para permitir su testeo (OCP, ISP).

2. **YAGNI - You Aren't Gonna Need It (Segunda Prioridad):**
   - No implementes microservicios complejos, colas de mensajería externas (como Kafka o RabbitMQ) ni infraestructuras en la nube sobre-diseñadas. 
   - Resuelve el problema con las herramientas estándar de Go (Channels, Mutex) y bases de datos relacionales estándar (SQLite/PostgreSQL) sin añadir abstracciones innecesarias o prematuras que el reto no solicita.

3. **KISS - Keep It Simple, Stupid (Tercera Prioridad):**
   - El código debe ser altamente legible. Evita patrones de concurrencia en Go que sean excesivamente "inteligentes" o difíciles de debuggear.
   - Si un `sync.Mutex` resuelve la condición de carrera de forma más clara que una red compleja de `Channels`, usa el Mutex, pero documenta por qué tomaste la decisión. 
   - Mantén la UI limpia y directa, enfocándote en la funcionalidad en tiempo real requerida.

4. **DRY - Don't Repeat Yourself (Cuarta Prioridad):**
   - Consolida la lógica de validación de pujas, las respuestas de error HTTP estándar, y las conexiones de base de datos.
   - Crea hooks personalizados o utilidades en el frontend (ej. `useWebSocket`, `useCountdown`) para evitar código duplicado en las vistas.

---

## 2. Requerimientos Funcionales y Técnicos a Implementar

### A. Backend (Go / Golang)
Desarrolla una API REST y un servidor WebSocket robusto con las siguientes características:

*   **API REST & Gestión de Subastas:**
    *   `GET /api/v1/auctions`: Catálogo de subastas (filtrando por activas, creadas, finalizadas).
    *   `POST /api/v1/auctions`: Endpoint protegido con JWT para crear subastas (Título, Precio Base, Imagen URL, Tiempo de Duración).
    *   `GET /api/v1/auctions/:id/bids`: Historial cronológico de pujas de un artículo.
*   **Motor Concurrente de Pujas (Core del Desafío):**
    *   **Prevención de Race Conditions:** Implementa una solución thread-safe utilizando `sync.Mutex`, `sync.RWMutex` o `Channels`. Debes garantizar que si dos o más peticiones llegan en el mismo milisegundo, solo se procese y acepte la primera/más alta, rechazando las demás.
    *   **Lógica de Rechazo:** Retorna un error claro (HTTP 400/409) si la puja entrante es menor al (precio actual + incremento mínimo configurado).
    *   **Cronómetro Automático:** Utiliza Goroutines con `time.Timer` o un bloque `select` para monitorear el tiempo de la subasta. En el milisegundo en que expira, la subasta debe cerrarse, mutar su estado en la DB a "finalizada" y declarar al ganador.
*   **Comunicación en Tiempo Real (WebSockets / SSE):**
    *   Ruta: `/api/v1/auctions/:id/ws`.
    *   Manejo de salas (rooms) independientes por ID de subasta.
    *   Emisión de eventos broadcast para: Nueva puja válida, Postor superado (Outbid), y Cierre de Subasta.
*   **Seguridad y Persistencia:**
    *   Implementa Middlewares para Autenticación JWT.
    *   Diseña el esquema en SQLite o PostgreSQL (con migraciones automáticas o scripts provistos) para Usuarios, Subastas e Historial de Pujas.

### B. Frontend (Astro + React)
Diseña la aplicación cliente aplicando la Arquitectura de Islas (Astro Islands):

*   **Renderizado Estático/Servidor (SSR/SSG):** 
    *   Construye la Landing Page, el Catálogo de Subastas y la Vista de Galería utilizando Astro puro. Asegura una carga instantánea y optimización SEO.
    *   Implementa un grid responsivo con filtros (Activas/Finalizadas).
*   **Isla Interactiva en React (Live Bidding Room):**
    *   Usa la directiva `client:load` o `client:only` para montar la vista de detalles de la subasta.
    *   **Componentes reactivos requeridos:**
        1.  Contador regresivo sincronizado con el backend.
        2.  Display dinámico del precio actual y el usuario que va ganando.
        3.  Formulario/Input para pujar, manejando el estado de carga y mostrando retroalimentación instantánea (toast notifications de éxito o error por puja baja).
        4.  Feed en directo (lista auto-scroll) con el historial de pujas entrantes vía WebSocket.

### C. DevOps, Entregables y Testing
Configura la infraestructura como código y las pruebas de la siguiente manera:

*   **Dockerización:**
    *   Crea un `docker-compose.yml` multiplataforma que levante 3 servicios: Backend Go, Frontend Astro y la Base de Datos (si se usa PostgreSQL).
    *   Asegúrate de usar Dockerfiles optimizados (multistage build para Go).
*   **Scripts de Pruebas de Estrés:**
    *   Desarrolla un script (`attack.sh`, `stress.py` o un pequeño cliente en Go) que dispare cientos de peticiones HTTP POST de pujas al mismo endpoint simultáneamente para probar empíricamente que tu Mutex/Channel previene las condiciones de carrera.
*   **Documentación (README.md):**
    *   Estructura clara de ejecución (`docker-compose up`).
    *   **Sección Crítica:** Escribe un apartado analítico detallando cómo se resolvieron las condiciones de carrera, por qué se eligió ese enfoque (Mutex vs Channels) y cómo se asegura el cierre exacto de la subasta.
*   **Preparación para la Demo:**
    *   Deja el código preparado de forma modular para facilitar la grabación del video demo (como se exige, donde se mostrarán dos pestañas compitiendo en tiempo real).

---

## 3. Plan de Ejecución (Para Claude Code)

Por favor, actúa de forma autónoma siguiendo estos pasos. Pide confirmación al terminar cada fase antes de avanzar:

1.  **Fase 1: Inicialización y Arquitectura.** Configura los repositorios, inicializa Go Modules, crea el proyecto Astro y define la estructura de carpetas (Clean Architecture simplificada). Crea los esquemas de base de datos.
2.  **Fase 2: Core Backend.** Desarrolla los modelos, repositorios, y el motor de subastas (implementando explícitamente el manejo de concurrencia y timers).
3.  **Fase 3: API y WebSockets.** Expón la lógica del core a través de los endpoints REST y configura el Hub/Salas de WebSockets.
4.  **Fase 4: Frontend.** Desarrolla el catálogo en Astro y la Isla Interactiva en React integrando la conexión al WebSocket.
5.  **Fase 5: DevOps & Documentación.** Escribe los Dockerfiles, el `docker-compose.yml`, el script de simulación de concurrencia y redacta el `README.md` final.

¡Comienza analizando este prompt y dime cómo estructurarás la base de datos y cuál será tu enfoque exacto para el manejo de concurrencia en Go antes de escribir la primera línea de código!
