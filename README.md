# GrabSeat

Real-time concurrent seat reservation system. When a popular event goes on sale
and hundreds of people grab for the same seats at once, GrabSeat guarantees that
exactly one person wins each seat — while everyone watching sees the seat map
update live.

Full documentation is being assembled. Quick start:

```bash
cp .env.example .env        # (Windows: copy .env.example .env)
docker compose up -d --build
```

Frontend: http://localhost:3000 · Backend gateway: http://localhost:8080
