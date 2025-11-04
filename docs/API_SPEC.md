# PumpUp — API Specification (MVP)

Base URL: http://{host}:{port} (по умолчанию http://localhost:8080)

---- Authentication / dev notes ----
* DEV AUTH: в демо используется заголовок X-User-Id (UUID) для идентификации пользователей.
* Endpoint /auth/login создаёт пользователя и возвращает user_id; сохраните его и используйте в X-User-Id.
* В продакшен заменить на JWT или OAuth.

---

POST /auth/login
Request:
  Content-Type: application/json
  Body:
    { "username": "string" }
Response 200:
  { "user_id": "uuid-string" }

Behavior:
  - Создаёт user + wallet с дефолтным балансом (demo).
  - Вернёт user_id — используйте как X-User-Id в последующих запросах.

---

GET /balance
Headers:
  X-User-Id: <uuid>
Response 200:
  { "balance": <integer> }   -- в минимальных денежных единицах (cents/kopek)

---

POST /rounds/start
Headers:
  X-User-Id: <uuid>
Body:
  {
    "stake": 1000,                     // integer
    "client_seed": "string",           // произвольная строка клиента для provably fair
    "difficulty": "easy|medium|hard"
  }
Response 200:
  {
    "round_id": "<uuid>",
    "server_seed_hash": "<hex>",   // sha256(serverSeed)
    "stake": 1000,
    "multiplier": 1.0,
    "step": 0
  }

Side effects:
  - Списывает stake с баланса пользователя (atomicity: в MVP — простая операция).
  - Создаёт запись rounds с server_seed (секретно) и server_seed_hash (публикуется).

Provably-fair:
  - server_seed_hash публикуется сразу (можно показать в UI).
  - server_seed раскрывается после cashout или поражения для верификации.

---

POST /rounds/step
Headers:
  X-User-Id: <uuid>
Body:
  {
    "round_id": "<uuid>",
    "difficulty": "easy|medium|hard"
  }
Response 200 (survived):
  {
    "result": "survived",
    "step": <int>,
    "multiplier": <float>
  }
Response 200 (exploded):
  {
    "result": "exploded",
    "step": <int>
  }

Behavior:
  - Берёт round.server_seed, round.client_seed, round.id и step index -> deterministic value (HMAC-SHA256).
  - Сравнивает с порогом вероятности survival для выбранной сложности.
  - Если survive -> увеличивает step и multiplier и логирует событие.
  - Если exploded -> помечает round.status='lost' и логирует.

---

POST /rounds/cashout
Headers:
  X-User-Id: <uuid>
Body:
  {
    "round_id": "<uuid>"
  }
Response 200:
  {
    "result": "cashedout",
    "payout": <int>,
    "server_seed": "<hex>",
    "server_seed_hash": "<hex>"
  }

Behavior:
  - Вычисляет payout = stake * multiplier (multiplier берётся из rounds).
  - Помечает round.status='cashedout', добавляет payout к балансу пользователя.
  - Возвращает server_seed для верификации пользователем.

---

GET /logs?admin=true
Headers:
  X-User-Id: <uuid>
Response:
  Array of log objects (id, round_id, user_id, event_type, payload, created_at)
Notes:
  - Требует admin parameter in MVP (для простоты). В продакшен заменить правами доступа.

---

# Provably Fair verification (клиент)

1. Получить `server_seed_hash` при старте раунда.
2. После cashout/explosion получить `server_seed`.
3. Проверить: sha256(server_seed) == server_seed_hash.
4. Вычислить HMAC-SHA256(server_seed, client_seed|roundId|stepIndex) и преобразовать в float 0..1.
   Если value < survivalProbability(step), шаг считался успешным. Повторить для каждого шага.
