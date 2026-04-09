# HowMuchYouSay - Specyfikacja projektu

## Przegląd

Internetowa gra cenowa, w której gracze zgadują i porównują ceny produktów z prawdziwych sklepów internetowych. Wyróżnik: dowolny sklep internetowy jako źródło danych, z ekstrakcją produktów przez agenta AI.

**Typ projektu:** Side project / hobby
**Stos:** React (frontend SPA) + Go/Gin (backend API) + PostgreSQL

---

## Tryby gry

### Tryb 1: Porównanie cen (Comparison)

1. Na ekranie pojawiają się zdjęcia i nazwy **dwóch produktów**
2. Gracz wybiera, który jest droższy (lub tańszy - losowo zmieniane **per runda**)
3. Odpowiedź: poprawna / niepoprawna

**Scoring (proporcjonalny do trudności):**

Różnica cenowa obliczana jako: `|cena_a - cena_b| / min(cena_a, cena_b) * 100%`

- Różnica cenowa > 50%: **1 punkt** (łatwe)
- Różnica 20-50%: **2 punkty** (średnie)
- Różnica 5-20%: **3 punkty** (trudne)
- Minimalna różnica cenowa między produktami w parze: 5%
- Błędna odpowiedź: 0 punktów

### Tryb 2: Zgadywanie ceny (Guess)

1. Na ekranie pojawia się zdjęcie i nazwa **jednego produktu**
2. Gracz wpisuje swoją cenę
3. Punkty zależą od trafności

**Scoring:**

Odchylenie obliczane jako: `|podana_cena - faktyczna_cena| / faktyczna_cena * 100%`

- Dokładne trafienie (odchylenie <= 2%): **5 punktów** (bonus)
- Odchylenie <= 10%: **3 punkty**
- Odchylenie <= 20%: **2 punkty**
- Odchylenie <= 30%: **1 punkt**
- Odchylenie > 30%: **0 punktów**

### Struktura sesji

- Gracz wybiera **jeden tryb** na sesję (porównywanie LUB zgadywanie)
- **10 rund** na sesję (domyślnie, konfigurowalnie w przyszłości)
- Rundy generowane z góry przed startem gry (losowy dobór produktów, bez powtórek)
- Po 10 rundach: ekran podsumowania z wynikiem

---

## Single Player

### Przepływ

**Z crawlowaniem (domyślnie):**
```
Gracz podaje nick + URL sklepu + wybiera tryb
  -> System crawluje sklep (gracz widzi progres)
  -> Pula produktów gotowa
  -> Runda 1 -> Odpowiedź -> Wynik rundy
  -> ...
  -> Runda 10 -> Podsumowanie (total score, statystyki per runda)
```

**Bez crawlowania (skip_crawl, dev/test):**
```
Gracz podaje nick + URL sklepu + wybiera tryb + skip_crawl=true
  -> System losuje produkty z istniejącej bazy sklepu (>= 20 wymagane)
  -> Runda 1 -> ...
  -> Runda 10 -> Podsumowanie
```

### Tożsamość

- Anonimowo - gracz podaje tylko nick
- Brak kont użytkowników, brak rejestracji
- Nic nie jest zapisywane między sesjami (z perspektywy gracza)

---

## Multiplayer (Real-time)

### Pokoje

- **Host** tworzy pokój: podaje nick, URL sklepu, tryb gry
- System generuje unikalny **6-znakowy kod alfanumeryczny** (np. `A3K9F2`)
- Gracze dołączają przez kod pokoju lub link: `howmuchyousay.com/room/A3K9F2`
- Gracz podaje nick przy dołączaniu
- **Max 8 graczy** na pokój
- Host decyduje o sklepie i trybie gry

### Lobby

- Lista graczy w pokoju (kto dołączył)
- Status crawlowania (ile produktów znaleziono)
- Host uruchamia grę gdy crawlowanie zakończone i gracze gotowi

### Komunikacja: WebSocket

Połączenie WS nawiązywane przy dołączeniu do pokoju. Wiadomości JSON:

| Event | Kierunek | Opis |
|-------|----------|------|
| `player_joined` | Server -> All | Nowy gracz w lobby |
| `player_left` | Server -> All | Gracz opuścił pokój |
| `crawl_status` | Server -> All | Progres crawlowania (products_found) |
| `game_start` | Server -> All | Gra rozpoczęta |
| `round_start` | Server -> All | Nowa runda + dane produktów |
| `answer_count` | Server -> All | Counter X/N odpowiedzi (bez ujawniania kto) |
| `round_result` | Server -> All | Wyniki rundy (odpowiedzi wszystkich + punkty) |
| `game_end` | Server -> All | Ranking końcowy |

### Bariera synchronizacji rund

1. Runda się zaczyna - wszyscy dostają pytanie jednocześnie
2. Każdy gracz odpowiada w swoim tempie
3. Na ekranie **counter X/N** (np. `3/5`) - ile osób odpowiedziało (bez ujawniania kto)
4. Czekamy aż wszyscy odpowiedzą **LUB timeout 30 sekund** per runda
5. Po barierze: odpowiedzi wszystkich + poprawna odpowiedź + punkty
6. Przerwa ~5 sekund na przegląd wyników
7. Następna runda

### Obsługa rozłączenia

- Timeout 15 sekund na reconnect po utracie połączenia
- Jeśli gracz nie wróci - "brak odpowiedzi" w rundzie (0 pkt), gra toczy się dalej
- Gracz może wrócić i dołączyć do bieżącej rundy

### Ranking końcowy

- Ranking graczy posortowany po punktach
- Statystyki per gracz (poprawne/błędne, najlepsza runda)
- Opcja "Zagraj ponownie" (ten sam pokój, ten sam sklep, nowe rundy z puli)

---

## Crawler / Agent ekstrakcji produktów

### Model AI

**GPT-5 mini** (OpenAI) - ekstrakcja strukturalna danych produktowych.

### Crawler jako niezależny komponent

Crawler jest **samodzielnym komponentem** - może działać niezależnie od gry. Dwa sposoby uruchomienia:

1. **CLI (osobny binary `crawler`)** - do ręcznego crawlowania i budowania bazy produktów:
   - `./crawler --url https://mediaexpert.pl` - crawluj sklep, zapisz produkty do bazy
   - Opcjonalne flagi: `--timeout 90s`, `--min-products 20`, `--verbose`
   - Wypisuje na stdout: progres + podsumowanie (ile produktów, ile stron, czas)
   - Logi do pliku per crawl_id (jak zawsze)
   - Crawl z CLI **nie ma powiązanej sesji gry** (`session_id` = null)

2. **Wewnętrznie przez serwer** - triggerowany automatycznie przy tworzeniu sesji gry (gdy `skip_crawl` = false). Używa tej samej logiki co CLI, ale uruchamiany jako goroutine.

### Przepływ crawlowania

1. Agent AI otrzymuje URL sklepu i analizuje stronę:
   - Sprawdza `robots.txt` (szanujemy zasady)
   - Pobiera HTML strony
   - Szuka ustrukturyzowanych danych: JSON-LD (`@type: Product`), microdata, Open Graph
   - Jeśli są - parsuje bezpośrednio (szybko, tanio)
   - Jeśli nie ma / za mało - AI analizuje HTML, identyfikuje linki do kategorii/produktów
2. Agent nawiguje po stronach produktowych, zbierając: **nazwa, cena, URL zdjęcia**
3. Cel: **20-30 produktów** (minimum na sesję gry, CLI może zebrać więcej)
4. Produkty zapisywane do bazy, powiązane ze sklepem i crawlem

### Limity i bezpieczeństwo

- **Timeout crawlowania:** 90 sekund max
- **Rate limiting:** max 1 request/sekundę do sklepu (grzeczne crawlowanie)
- **Minimum produktów:** jeśli < 20 po crawlowaniu, sprawdzamy cache (patrz: Cache produktów)

### Walidacja danych

- Cena musi być liczbą > 0
- Nazwa nie może być pusta
- URL zdjęcia musi być prawidłowy (fallback na placeholder)
- Duplikaty usuwane po nazwie (normalizacja)

### Cache produktów (reuse między sesjami)

- Produkty powiązane ze sklepem (nie z sesją gry)
- **Zawsze próbujemy crawlować od nowa** przy każdej sesji
- Jeśli crawl się powiedzie -> nowe produkty do bazy, gra z nowych produktów
- Jeśli crawl zawiedzie -> fallback na historyczne produkty z bazy dla tego sklepu
  - Jeśli >= 20 produktów w historii -> losowa selekcja, gra możliwa
  - Jeśli < 20 -> komunikat błędu, gra nie może się rozpocząć
- Z czasem per sklep narastają produkty z różnych crawli -> bogatsza pula

### Logowanie

Każde zadanie crawlowania generuje szczegółowy plik logów:

- **Jeden plik per crawl:** `logs/crawl_<crawl_id>.log`
- Każdy krok agenta logowany z timestampem:
  - `FETCH` - pobranie URL, status HTTP, rozmiar odpowiedzi
  - `PARSE` - co znaleziono (JSON-LD? microdata? nic?)
  - `AI_REQUEST` - prompt wysłany do modelu, liczba tokenów
  - `AI_RESPONSE` - odpowiedź modelu, sparsowane produkty
  - `NAVIGATE` - decyzja agenta o przejściu na kolejną stronę + powód
  - `PRODUCT_FOUND` - wyekstrahowany produkt (nazwa, cena)
  - `VALIDATION` - wynik walidacji (odrzucone produkty + powód)
  - `ERROR` - każdy błąd z pełnym kontekstem
- Ścieżka do pliku logów zapisana w tabeli `crawls`

---

## Model danych (PostgreSQL)

### shops

| Kolumna | Typ | Opis |
|---------|-----|------|
| id | UUID, PK | |
| url | TEXT, UNIQUE | Znormalizowany URL bazowy sklepu |
| name | TEXT, nullable | Nazwa sklepu (opcjonalna) |
| first_crawled_at | TIMESTAMP | |
| last_crawled_at | TIMESTAMP | |

### crawls

| Kolumna | Typ | Opis |
|---------|-----|------|
| id | UUID, PK | crawl_id |
| shop_id | UUID, FK -> shops | |
| session_id | UUID, FK -> game_sessions, **nullable** | Null gdy crawl odpalony z CLI |
| status | ENUM | pending, in_progress, completed, failed |
| products_found | INT | Ile produktów wyekstrahowano |
| pages_visited | INT | Ile stron odwiedzono |
| ai_requests_count | INT | Ile zapytań do LLM |
| error_message | TEXT, nullable | Powód błędu |
| log_file_path | TEXT | Ścieżka do pliku logów |
| started_at | TIMESTAMP | |
| finished_at | TIMESTAMP, nullable | |
| duration_ms | INT, nullable | Czas trwania |

### products

| Kolumna | Typ | Opis |
|---------|-----|------|
| id | UUID, PK | |
| shop_id | UUID, FK -> shops | Produkt należy do sklepu |
| crawl_id | UUID, FK -> crawls | Z którego crawla pochodzi |
| name | TEXT | |
| price | DECIMAL | |
| image_url | TEXT | |
| source_url | TEXT | URL strony produktu w sklepie |
| created_at | TIMESTAMP | |

### game_sessions

| Kolumna | Typ | Opis |
|---------|-----|------|
| id | UUID, PK | |
| room_code | VARCHAR(6), nullable | Null dla single player |
| host_nick | VARCHAR | |
| shop_id | UUID, FK -> shops | |
| game_mode | ENUM | comparison, guess |
| rounds_total | INT | Domyślnie 10 |
| status | ENUM | crawling, ready, lobby, in_progress, finished. Single: crawling->ready->in_progress->finished (skip_crawl pomija crawling). Multi: crawling->lobby->in_progress->finished. |
| crawl_id | UUID, FK -> crawls, **nullable** | Null gdy gra z istniejącej bazy (skip_crawl) |
| created_at | TIMESTAMP | |
| updated_at | TIMESTAMP | |

### players

| Kolumna | Typ | Opis |
|---------|-----|------|
| id | UUID, PK | |
| session_id | UUID, FK -> game_sessions | |
| nick | VARCHAR | |
| joined_at | TIMESTAMP | |
| is_host | BOOLEAN | |

### rounds

| Kolumna | Typ | Opis |
|---------|-----|------|
| id | UUID, PK | |
| session_id | UUID, FK -> game_sessions | |
| round_number | INT | |
| round_type | ENUM | comparison, guess |
| product_a_id | UUID, FK -> products | |
| product_b_id | UUID, FK -> products, nullable | Null w trybie guess |
| correct_answer | TEXT | "a"/"b" dla comparison, cena dla guess |
| difficulty_score | INT | Punkty do zdobycia (1/2/3) |

### answers

| Kolumna | Typ | Opis |
|---------|-----|------|
| id | UUID, PK | |
| round_id | UUID, FK -> rounds | |
| player_id | UUID, FK -> players | |
| answer | TEXT | "a"/"b" lub podana cena |
| is_correct | BOOLEAN | |
| points_earned | INT | |
| answered_at | TIMESTAMP | |

---

## API

### REST Endpoints

**Gra (single player):**
- `POST /api/game` - Utwórz sesję `{nick, shop_url, game_mode, skip_crawl?}` -> `{session_id}`
  - `skip_crawl: false` (domyślnie): triggeruje crawlowanie, status sesji = `crawling`
  - `skip_crawl: true`: pomija crawlowanie, losuje produkty z istniejącej bazy sklepu. Wymaga >= 20 produktów w bazie dla danego `shop_url`. Status sesji od razu = `in_progress`
- `GET /api/game/:session_id` - Status sesji (crawling/ready/in_progress/finished) + dane gry
  - Używane do pollowania statusu crawlowania
- `GET /api/game/:session_id/round/:number` - Dane rundy (produkty)
- `POST /api/game/:session_id/round/:number/answer` - Odpowiedź gracza `{answer}` -> `{is_correct, points, correct_answer}`
- `GET /api/game/:session_id/results` - Podsumowanie końcowe

**Pokoje (multiplayer):**
- `POST /api/room` - Utwórz pokój `{host_nick, shop_url, game_mode, skip_crawl?}` -> `{room_code, session_id}`
  - `skip_crawl` działa jak w single player
- `POST /api/room/:code/join` - Dołącz `{nick}` -> `{player_id}`
- `GET /api/room/:code` - Info o pokoju (gracze, status)

### WebSocket

- `WS /ws/room/:code` - Połączenie do pokoju multiplayer
- Autoryzacja po połączeniu: `{player_id}` (z POST /join)
- Komunikacja event-driven (patrz: tabela eventów w sekcji Multiplayer)

---

## Architektura

**Podejście:** Monolit z wydzielonymi wewnętrznymi pakietami. Dwa osobne binary dzielące wspólny kod.

### Binary

1. **`server`** - serwer HTTP:
   - REST API (Gin)
   - WebSocket hub (goroutine per pokój)
   - Crawlowanie on-demand (goroutine per zadanie, triggerowane przez start gry)
   - Dostęp do bazy (PostgreSQL)

2. **`crawler`** - CLI do crawlowania:
   - `./crawler --url <shop_url>` - crawluj sklep i zapisz produkty do bazy
   - Flagi: `--timeout`, `--min-products`, `--verbose`
   - Używa tych samych pakietów `internal/crawler/` i `internal/store/` co serwer
   - Niezależny od serwera - wymaga tylko połączenia do bazy

Oba binary współdzielą pakiety z `internal/` (crawler, store, models, config).

Uzasadnienie: side project, Go dobrze obsługuje concurrency. Jeśli projekt urośnie, wydzielenie crawlera jako osobnego workera to naturalny następny krok.

---

## Struktura plików

```
howmuchyousay/
├── frontend/                    # React SPA
│   ├── public/
│   ├── src/
│   │   ├── api/                 # Klient HTTP + WebSocket
│   │   │   ├── client.ts        # Axios/fetch wrapper
│   │   │   ├── gameApi.ts       # Endpointy gry
│   │   │   ├── roomApi.ts       # Endpointy pokojów
│   │   │   └── websocket.ts     # WebSocket client
│   │   ├── components/          # Komponenty UI
│   │   │   ├── common/          # Buttony, inputy, loader
│   │   │   ├── game/            # Komponenty rozgrywki
│   │   │   │   ├── ComparisonRound.tsx
│   │   │   │   ├── GuessRound.tsx
│   │   │   │   ├── ProductCard.tsx
│   │   │   │   ├── RoundResult.tsx
│   │   │   │   ├── ScoreBoard.tsx
│   │   │   │   └── Timer.tsx
│   │   │   ├── lobby/           # Lobby multiplayer
│   │   │   │   ├── PlayerList.tsx
│   │   │   │   ├── RoomCode.tsx
│   │   │   │   └── CrawlProgress.tsx
│   │   │   └── setup/           # Ekrany konfiguracji
│   │   │       ├── GameSetup.tsx # Podanie URL, wybór trybu
│   │   │       ├── NickInput.tsx
│   │   │       └── JoinRoom.tsx
│   │   ├── pages/               # Strony (routing)
│   │   │   ├── HomePage.tsx
│   │   │   ├── SinglePlayerPage.tsx
│   │   │   ├── MultiPlayerPage.tsx
│   │   │   ├── GamePage.tsx
│   │   │   └── ResultsPage.tsx
│   │   ├── hooks/               # Custom hooks
│   │   │   ├── useGame.ts       # Logika gry SP
│   │   │   ├── useRoom.ts       # Logika pokoju MP
│   │   │   └── useWebSocket.ts  # Hook WS
│   │   ├── types/               # TypeScript typy
│   │   │   └── index.ts
│   │   ├── utils/               # Helpery
│   │   ├── App.tsx
│   │   └── main.tsx
│   ├── package.json
│   ├── tsconfig.json
│   └── vite.config.ts
│
├── backend/                     # Go (Gin) API
│   ├── cmd/
│   │   ├── server/
│   │   │   └── main.go          # Entry point serwera HTTP
│   │   └── crawler/
│   │       └── main.go          # Entry point CLI crawlera
│   ├── internal/
│   │   ├── api/                 # HTTP handlers
│   │   │   ├── router.go        # Gin router setup
│   │   │   ├── game_handler.go  # /api/game endpoints
│   │   │   ├── room_handler.go  # /api/room endpoints
│   │   │   └── middleware.go    # CORS, logging, recovery
│   │   ├── ws/                  # WebSocket
│   │   │   ├── hub.go           # Room hub manager
│   │   │   ├── client.go        # WS client connection
│   │   │   └── messages.go      # Message types
│   │   ├── game/                # Logika gry
│   │   │   ├── engine.go        # Silnik gry (rundy, scoring)
│   │   │   ├── comparison.go    # Logika trybu porównywania
│   │   │   ├── guess.go         # Logika trybu zgadywania
│   │   │   └── round_gen.go     # Generowanie rund z puli produktów
│   │   ├── crawler/             # Agent crawlujący
│   │   │   ├── crawler.go       # Główna logika crawlowania
│   │   │   ├── agent.go         # AI agent (nawigacja, decyzje)
│   │   │   ├── extractor.go     # Ekstrakcja danych (JSON-LD, microdata)
│   │   │   ├── ai_client.go     # Klient OpenAI (GPT-5 mini)
│   │   │   ├── validator.go     # Walidacja produktów
│   │   │   └── logger.go        # Logowanie crawlowania do plików
│   │   ├── store/               # Warstwa danych (PostgreSQL)
│   │   │   ├── db.go            # Połączenie DB
│   │   │   ├── shop_store.go
│   │   │   ├── product_store.go
│   │   │   ├── game_store.go
│   │   │   ├── round_store.go
│   │   │   ├── player_store.go
│   │   │   └── crawl_store.go
│   │   ├── models/              # Struktury danych
│   │   │   ├── game.go
│   │   │   ├── product.go
│   │   │   ├── player.go
│   │   │   ├── round.go
│   │   │   ├── shop.go
│   │   │   └── crawl.go
│   │   └── config/              # Konfiguracja
│   │       └── config.go        # Env vars, defaults
│   ├── migrations/              # SQL migracje
│   │   ├── 001_create_shops.sql
│   │   ├── 002_create_crawls.sql
│   │   ├── 003_create_products.sql
│   │   ├── 004_create_game_sessions.sql
│   │   ├── 005_create_players.sql
│   │   ├── 006_create_rounds.sql
│   │   └── 007_create_answers.sql
│   ├── logs/                    # Logi crawlowania (gitignored)
│   │   └── crawl_<crawl_id>.log
│   ├── go.mod
│   └── go.sum
│
├── docker-compose.yml           # PostgreSQL + backend + frontend
├── .env.example                 # OPENAI_API_KEY, DB_URL, etc.
├── Makefile                     # make run, make migrate, etc.
└── README.md
```

---

## Przyszłe rozszerzenia (poza scope MVP)

- Konfigurowalny liczba rund
- Tryb mix (losowe mieszanie porównywania i zgadywania w sesji)
- Wybór kategorii produktów przed grą (po analizie sklepu)
- Konta użytkowników z historią gier i statystykami
- Publiczny matchmaking
- Tryb endless / na czas / system życiań
- Predefiniowana lista popularnych sklepów
