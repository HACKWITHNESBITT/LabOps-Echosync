# EchoSync 

> **Ambient Intelligence & Real-Time Proximity Matching Platform**

EchoSync is a privacy-first, low-latency proximity discovery system that passively listens to ambient conversations, sanitizes personal identifiable information (PII) on-device, extracts vector interest tokens, and matches nearby users in real-time within a 15-meter radius.

---

## Key Features

* **On-Device PII Scrubbing:** Utilizes local LLMs (Llama-3-8B) to redact sensitive data before any vector embedding is processed.
* **15-Meter Match Radar:** Real-time spatial tracking and visual sonar interface displaying passive and active connections.
* **Ephemeral Match Feed:** Self-destructing match cards governed by a strict 15-minute Time-To-Live (TTL).
* **AI-Generated Icebreakers:** Contextual, dynamic prompt suggestions designed to initiate immediate, real-world conversations.
* **Haptic Proximity Feedback:** Subtle physical cues sent to mobile clients upon detecting high-similarity interest vectors.

---

##  Architecture & Technology Stack

### **Frontend & Dashboard**
* **Mobile App:** Built with **Flutter** (Cross-platform iOS/Android) featuring native haptic engine integration and custom radial canvas drawing.
* **Command Center:** Web dashboard built with **React / Next.js**, **Tailwind CSS**, and **Lucide Icons** for operational monitoring.

### **Backend & AI Pipeline**
* **Core API:** High-performance REST & WebSocket server built with **Go (Golang)**.
* **Privacy Firewall:** Local **Llama-3-8B** inference engine running on-device or via edge worker node for zero-trust transcript scrubbing.
* **Proximity Engine:** Geo-fencing & spatial vector indexing using **Redis Geo** and **pgvector** (PostgreSQL).

---

##  Wireframe & UI Overview

| Desktop Command Center | Mobile Radar & Icebreaker View |
| :---: | :---: |
| *Multi-panel operations dashboard featuring live PII pipeline, spatial radar, and ephemeral feed.* | *Flutter mobile layout showing real-time proximity radar and generated icebreaker prompts.* |

---

##  Quick Start & Installation

### Prerequisites
* **Go** `1.22+`
* **Flutter SDK** `3.19+`
* **Node.js** `20+` & **pnpm** (`npm` works too)
* **Docker** & **Docker Compose**

### 1. Environment configuration
```bash
cp .env.example .env
```

The Go server reads its configuration from environment variables, so export them
before running the backend:

```bash
set -a; . ./.env; set +a
```

> **Ollama (Llama-3-8B) is optional.** When unreachable, the server automatically
> falls back to the deterministic rule-based PII firewall and template icebreakers.

### 2. Static analysis
```bash
flutter pub get
flutter analyze
```

### 3. Start the infrastructure
```bash
docker compose up -d postgres redis   # postgres :5544, redis :5545
```

### 4. Start the backend
```bash
set -a; . ./.env; set +a
cd backend && go run ./cmd/server     # listens on :8080
```

Verify it is up:

```bash
curl http://127.0.0.1:8080/health
```

### 5. Start the command center (Next.js / React)
```bash
cd web
npm install
npm run build
npm start                             # http://localhost:3000
```

The command center talks to the backend at `127.0.0.1:8080` by default. Override it
with `NEXT_PUBLIC_API_URL` **before** running `npm run build`.

### 6. Start the Flutter app
One-time web platform setup (adds Flutter web files next to the Next.js command
center — do not run `flutter create .` again after this):

```bash
flutter create . --platforms web
```

Build and serve:

```bash
flutter build web --release
python3 -m http.server 3001 --directory build/web   # http://localhost:3001
```

Or run on a connected device/emulator with `flutter run`.

Live mic capture: the mic button streams pcm_s16le @ 16 kHz mono to the
backend's `WS /v1/audio/stream` and shows live captions. Mic streaming works
on Android/iOS/desktop builds; on Flutter **web** (browser cannot emit raw
PCM) the button shows a hint and the radar demo still runs.

### 7. End-to-end demo
```bash
bash scripts/demo.sh   # requires the backend on :8080 and jq
```

### 7b. Audio pipeline hands-on (WAV upload)
```bash
# Transcribe an uploaded clip -> scrub PII -> register presence -> match:
curl -XPOST 'http://127.0.0.1:8080/v1/audio?user_id=kim&lat=-1.282&lng=36.821' \
  -H 'Content-Type: audio/wav' --data-binary @/path/to/speech.wav
```
- One-shot uploads are transcribed with Groq Whisper (`whisper-large-v3-turbo`,
  falls back to a Speechmatics short session for raw PCM).
- Live streaming (mic, WS) runs through Speechmatics real-time
  (`wss://eu.rt.speechmatics.com/v2`) with final transcripts debounced and
  fed into the same scrub -> presence -> match engine.
- `GROQ_MODEL` selects the cloud Llama-class firewall (default
  `openai/gpt-oss-120b`); `ECHO_PII_MODE=auto` prefers it, then Ollama/rule.
- Privacy pipeline events stream over `/ws?user_id=dashboard` and
  `/v1/pipeline/events` for the command center.

### Service summary
| Service              | URL / Port        |
| -------------------- | ----------------- |
| Backend API          | http://127.0.0.1:8080 |
| Next.js command center | http://127.0.0.1:3000 |
| Flutter app (web)    | http://127.0.0.1:3001 |
| PostgreSQL           | localhost:5544    |
| Redis                | localhost:5545    |
