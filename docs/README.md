# PumpUp 

Требования:
- Docker & docker-compose
- (локально) Go 1.20 и Node 18 (необязательно, docker соберёт всё)

Запуск:
1) Клонируйте репозиторий в /path/to/pumpup
2) Перейдите в папку:
   cd pumpup

3) (опционально) отредактируйте backend/.env.example и docker-compose env

4) Запустите docker-compose:
   docker-compose up --build

Сервисы:
- Backend API: http://localhost:8080
- Frontend dev: http://localhost:5173

Dev flow:
- Создайте demo пользователя:
  POST http://localhost:8080/auth/login { "username": "ivan" }
  => получите user_id

- Используйте X-User-Id: <user_id> в заголовках при запросах.

DB init:
- db.sql автоматически монтируется и выполняется при старте Postgres (docker-entrypoint-initdb.d).

Рекомендации:
- В production заменить X-User-Id на JWT.
- Добавить транзакционность в списание/выдачу баланса.
- Валидацию idempotency-key для start/step/cashout.
