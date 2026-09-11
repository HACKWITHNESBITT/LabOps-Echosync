# EchoSync 🎙️⚡

> **Ambient Intelligence & Real-Time Proximity Matching Platform**

EchoSync is a privacy-first, low-latency proximity discovery system that passively listens to ambient conversations, sanitizes personal identifiable information (PII) on-device, extracts vector interest tokens, and matches nearby users in real-time within a 15-meter radius.

---

## 🌟 Key Features

* **On-Device PII Scrubbing:** Utilizes local LLMs (Llama-3-8B) to redact sensitive data before any vector embedding is processed.
* **15-Meter Match Radar:** Real-time spatial tracking and visual sonar interface displaying passive and active connections.
* **Ephemeral Match Feed:** Self-destructing match cards governed by a strict 15-minute Time-To-Live (TTL).
* **AI-Generated Icebreakers:** Contextual, dynamic prompt suggestions designed to initiate immediate, real-world conversations.
* **Haptic Proximity Feedback:** Subtle physical cues sent to mobile clients upon detecting high-similarity interest vectors.

---

## 🏗️ Architecture & Technology Stack

### **Frontend & Dashboard**
* **Mobile App:** Built with **Flutter** (Cross-platform iOS/Android) featuring native haptic engine integration and custom radial canvas drawing.
* **Command Center:** Web dashboard built with **React / Next.js**, **Tailwind CSS**, and **Lucide Icons** for operational monitoring.

### **Backend & AI Pipeline**
* **Core API:** High-performance REST & WebSocket server built with **Go (Golang)**.
* **Privacy Firewall:** Local **Llama-3-8B** inference engine running on-device or via edge worker node for zero-trust transcript scrubbing.
* **Proximity Engine:** Geo-fencing & spatial vector indexing using **Redis Geo** and **pgvector** (PostgreSQL).

---

## 📸 Wireframe & UI Overview

| Desktop Command Center | Mobile Radar & Icebreaker View |
| :---: | :---: |
| *Multi-panel operations dashboard featuring live PII pipeline, spatial radar, and ephemeral feed.* | *Flutter mobile layout showing real-time proximity radar and generated icebreaker prompts.* |

---

## 🚀 Quick Start & Installation

### Prerequisites
* **Go** `1.22+`
* **Flutter SDK** `3.19+`
* **Node.js** `20+` & **pnpm**
* **Docker** & **Docker Compose**
