# Deferred work

Список технических долгов и hardening-возможностей, обнаруженных в review-итерациях, но сознательно вынесенных за рамки текущих PR. Каждая запись — кандидат на отдельный issue/spec.

---

## GH-39 review (2026-05-01) — header-trust hardening

Спека `spec-gh-39-real-client-ip.md` явно через `Ask First` гейтила любые application-уровневые механизмы доверия заголовкам. Reviewers (blind-hunter + edge-case-hunter) подняли несколько связанных идей — все откладываются как **отдельная security-задача**:

- **Reject loopback / private / unspecified / link-local IPs** в заголовках. `net.ParseIP("0.0.0.0")` / `127.0.0.1` / `169.254.169.254` / `::` сейчас принимаются как валидные. Реальный CF-IP всегда publicly routable; defense-in-depth — отбрасывать non-public ranges и fall through на следующий header.
- **Multiple header values detection.** `r.Header.Get(h)` возвращает только первое значение. Атакующий с двумя `CF-Connecting-IP` заголовками может вытеснить настоящий. Использовать `r.Header.Values(h)` либо явно отвергать запрос при `len > 1`.
- **Empty-string sentinel для `audit_logs.ip_address`.** `NOT NULL TEXT` не ловит пустую строку. Если все источники пусты/невалидны и `r.RemoteAddr` пуст (теоретически в синтетических транспортах), audit получит `""`. Подставлять документированный sentinel (`"unknown"`/`"-"`).
- **CIDR allowlist для `r.RemoteAddr`.** Триггер любого header-trust только если непосредственный peer (Dokploy) попадает в ожидаемый CIDR. Закрывает класс «firewall misconfig + IPv6 leak».

**Trigger:** Если будет инцидент, где `audit_logs.ip_address` окажется attacker-controlled — поднимать всё перечисленное одним PR.

---

## GH-39 review (2026-05-01) — XFF tolerance

- **Skip-empty leading tokens в X-Forwarded-For.** При `X-Forwarded-For: ", , 1.2.3.4"` текущая реализация берёт первый токен (`""`) и падает на fallback вместо итерации до первого валидного IP. CF/Dokploy не производят такой формат, но spec-замок «leftmost client IP» формально не выполняется на dirty input.
- **Tolerate `host:port` в XFF.** RFC 7239 запрещает, но dirty proxies иногда добавляют порт. Сейчас `1.2.3.4:5678` отвергается ParseIP'ом. Можно сделать `SplitHostPort` fallback.

**Trigger:** появление нестандартного прокси в цепочке (новый CDN, mid-box, экспериментальный edge).

---

## GH-39 review (2026-05-01) — middleware refactor

- **Слить `RealIP` + `ClientIP` в один middleware.** Сейчас два уровня очистки: `RealIP` пишет резолвленный host в `r.RemoteAddr`, `ClientIP` ещё раз `SplitHostPort`'ит и кладёт в context. Работает, но дублирует логику и хрупко (если кто-то добавит валидацию в `RealIP` без trim'а порта, `ClientIP` сделает это незаметно).

**Trigger:** при следующей серьёзной правке IP-resolution chain — переделать заодно.
