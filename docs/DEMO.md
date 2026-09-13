# Demo

## The 60–90 second screen recording

Record this once the stack is running, then convert to a GIF so it renders inline
in the README (`docs/media/demo.gif`) and keep the full video linked separately if
you want a longer cut.

For a snappy recording, run with a short hold TTL so the expiry moment happens on
camera:

```bash
# .env: set HOLD_TTL_SECONDS=15
docker compose up -d --build
```

### Shot list

1. **Two tabs, same seat, at once (~15s).** Open http://localhost:3000 in two
   tabs side by side. Click the same open seat in both at nearly the same instant.
   One tab turns the seat gold ("your hold"); the other shows the toast that
   someone grabbed it first and the seat as amber.
2. **A hold expiring live (~15s).** In the winning tab, don't confirm. Let the
   countdown run out. Both tabs show the seat return to open the moment it expires.
3. **A successful confirm (~15s).** Grab a seat again and click **Confirm
   booking** before the timer runs out. Both tabs show it lock in as booked (the
   check mark).
4. **Zero double-bookings under load (~15s).** Cut to a terminal and run the
   concurrency test, showing the result line:

   ```bash
   docker run --rm --network=grabseat_default \
     -e RUN_INTEGRATION=1 -e BASE_URL=http://gateway:8080 -e WS_URL=ws://gateway:8080/ws \
     -v "$(pwd)/loadtest:/src" -w /src golang:1.23-alpine \
     go test -v -run TestConcurrencyStress ./...
   ```

   The line to land on: `successes=10 rejections=140 double_bookings=0`.

### Converting to a GIF

Any screen recorder works. To turn an `.mp4`/`.mov` into a README-friendly GIF
with ffmpeg:

```bash
ffmpeg -i recording.mp4 -vf "fps=12,scale=900:-1:flags=lanczos" -loop 0 docs/media/demo.gif
```

Keep it under a few MB so it loads fast on GitHub. Drop the file at
`docs/media/demo.gif` and the README picks it up automatically.

## Live demo checklist (for showing someone in person)

- [ ] `docker compose up -d --build` from a clean clone, nothing else installed.
- [ ] Open two tabs; race for the same seat; exactly one wins.
- [ ] Let a hold expire; watch it free up live in both tabs.
- [ ] Confirm a booking; watch it lock permanently in both tabs.
- [ ] `docker compose logs -f backend` — point at a trace id following one booking
      and the OpenTelemetry spans around lock + confirm.
- [ ] Run the concurrency test; show `double_bookings=0`.
- [ ] (Optional) `docker compose up -d --scale backend=2` and run the
      multi-instance test to show a booking on one instance reaching a client on
      another.
- [ ] (Optional) Stop Redis mid-load (`bash loadtest/redis_resilience.sh`) to show
      clean degradation and recovery.
