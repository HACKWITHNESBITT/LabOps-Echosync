"use client";

import { useEffect, useRef, useState } from "react";
import {
  Mic,
  MicOff,
  Volume2,
  VolumeX,
  Wifi,
  Sparkles,
  Send,
  Monitor,
  Layers,
  RefreshCw,
  Maximize2,
  Minimize2,
  Loader2,
  ShieldCheck,
} from "lucide-react";

interface VoiceInterfaceProps {
  onClose?: () => void;
}

// Pure JS WAV encoder (16-bit PCM mono)
function writeString(view: DataView, offset: number, string: string) {
  for (let i = 0; i < string.length; i++) {
    view.setUint8(offset + i, string.charCodeAt(i));
  }
}

function encodeWAV(samples: Float32Array, sampleRate: number): Blob {
  const numChannels = 1;
  const bitDepth = 16;
  const bytesPerSample = bitDepth / 8;
  const blockAlign = numChannels * bytesPerSample;
  const dataLength = samples.length * bytesPerSample;
  const buffer = new ArrayBuffer(44 + dataLength);
  const view = new DataView(buffer);

  writeString(view, 0, "RIFF");
  view.setUint32(4, 36 + dataLength, true);
  writeString(view, 8, "WAVE");
  writeString(view, 12, "fmt ");
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true); // PCM format
  view.setUint16(22, numChannels, true);
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, sampleRate * blockAlign, true);
  view.setUint16(32, blockAlign, true);
  view.setUint16(34, bitDepth, true);
  writeString(view, 36, "data");
  view.setUint32(40, dataLength, true);

  let offset = 44;
  for (let i = 0; i < samples.length; i++) {
    const s = Math.max(-1, Math.min(1, samples[i]));
    view.setInt16(offset, s < 0 ? s * 0x8000 : s * 0x7fff, true);
    offset += 2;
  }
  return new Blob([view], { type: "audio/wav" });
}

function downsampleTo16k(
  input: Float32Array,
  inputSampleRate: number,
): Float32Array {
  if (inputSampleRate === 16000) return input;
  const ratio = inputSampleRate / 16000;
  const newLength = Math.round(input.length / ratio);
  const result = new Float32Array(newLength);
  for (let i = 0; i < newLength; i++) {
    const origIdx = i * ratio;
    const idx = Math.floor(origIdx);
    const frac = origIdx - idx;
    const nextIdx = Math.min(idx + 1, input.length - 1);
    result[i] = input[idx] * (1 - frac) + input[nextIdx] * frac;
  }
  return result;
}

export function VoiceInterface({ onClose }: VoiceInterfaceProps) {
  const [viewMode, setViewMode] = useState<"interactive" | "monitor">(
    "interactive",
  );
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [isRecording, setIsRecording] = useState(false);
  const [isProcessing, setIsProcessing] = useState(false);
  const [isSpeaking, setIsSpeaking] = useState(false);
  const [speechEnabled, setSpeechEnabled] = useState(true);

  // Real user query & AI response (no mock data)
  const [query, setQuery] = useState("");
  const [response, setResponse] = useState(
    "EchoSync voice core ready. Click the microphone to test with your voice, or type a query.",
  );
  const [inputVal, setInputVal] = useState("");
  const [audioLevel, setAudioLevel] = useState(0);
  const [currentTime, setCurrentTime] = useState("");

  // Live telemetry from actual speech processing
  const [extractedTokens, setExtractedTokens] = useState<string[]>([]);
  const [blockedPii, setBlockedPii] = useState<string[] | null>(null);
  const [pipelineLatency, setPipelineLatency] = useState<number | null>(null);
  const [micStatusText, setMicStatusText] = useState("Click to Activate");
  const [micError, setMicError] = useState<string | null>(null);

  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const enteredFromMonitorRef = useRef(false);

  // Audio recording refs
  const audioCtxRef = useRef<AudioContext | null>(null);
  const analyserRef = useRef<AnalyserNode | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const scriptProcessorRef = useRef<ScriptProcessorNode | null>(null);
  const audioChunksRef = useRef<Float32Array[]>([]);
  const isRecordingRef = useRef(false);
  const speechRecognitionRef = useRef<any>(null);

  // Live clock
  useEffect(() => {
    const updateTime = () => {
      const now = new Date();
      setCurrentTime(
        now.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
      );
    };
    updateTime();
    const timer = setInterval(updateTime, 10000);
    return () => clearInterval(timer);
  }, []);

  // Text-to-speech helper
  const speakText = (text: string) => {
    if (!speechEnabled || typeof window === "undefined" || !("speechSynthesis" in window))
      return;
    try {
      window.speechSynthesis.cancel();
      const utterance = new SpeechSynthesisUtterance(text);
      utterance.rate = 1.05;
      utterance.pitch = 1.0;
      utterance.onstart = () => setIsSpeaking(true);
      utterance.onend = () => setIsSpeaking(false);
      utterance.onerror = () => setIsSpeaking(false);

      const voices = window.speechSynthesis.getVoices();
      const preferred =
        voices.find(
          (v) =>
            v.lang.startsWith("en") &&
            (v.name.includes("Natural") ||
              v.name.includes("Google") ||
              v.name.includes("Samantha")),
        ) || voices.find((v) => v.lang.startsWith("en"));

      if (preferred) utterance.voice = preferred;
      window.speechSynthesis.speak(utterance);
    } catch {
      setIsSpeaking(false);
    }
  };

  // Fullscreen handlers
  const enterFullscreen = (fromMonitor: boolean = false) => {
    enteredFromMonitorRef.current = fromMonitor;
    setIsFullscreen(true);
    setViewMode("interactive");
    if (typeof document !== "undefined") {
      try {
        if (
          !document.fullscreenElement &&
          containerRef.current?.requestFullscreen
        ) {
          containerRef.current.requestFullscreen().catch(() => {});
        }
      } catch {
        // Fallback to CSS fullscreen
      }
    }
  };

  const exitFullscreen = () => {
    setIsFullscreen(false);
    if (enteredFromMonitorRef.current) {
      setViewMode("monitor");
      enteredFromMonitorRef.current = false;
    }
    if (typeof document !== "undefined" && document.fullscreenElement) {
      try {
        document.exitFullscreen().catch(() => {});
      } catch {
        // Ignore
      }
    }
  };

  useEffect(() => {
    const handleFullscreenChange = () => {
      if (!document.fullscreenElement && isFullscreen) {
        setIsFullscreen(false);
        if (enteredFromMonitorRef.current) {
          setViewMode("monitor");
          enteredFromMonitorRef.current = false;
        }
      }
    };

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape" && isFullscreen) {
        exitFullscreen();
      }
    };

    document.addEventListener("fullscreenchange", handleFullscreenChange);
    window.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("fullscreenchange", handleFullscreenChange);
      window.removeEventListener("keydown", handleKeyDown);
    };
  }, [isFullscreen]);

  // Real-time canvas waveform renderer & audio level meter
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    let animId: number;

    const render = () => {
      const width = canvas.width;
      const height = canvas.height;
      const centerY = height / 2;
      ctx.clearRect(0, 0, width, height);

      // Neon Gradient (cyan -> emerald -> pink)
      const grad = ctx.createLinearGradient(0, 0, width, 0);
      grad.addColorStop(0, "#00f2fe");
      grad.addColorStop(0.5, "#38ef7d");
      grad.addColorStop(0.75, "#e7839a");
      grad.addColorStop(1, "#ff007f");

      ctx.shadowBlur = 12;
      ctx.shadowColor = "#00f2fe";

      const analyser = analyserRef.current;
      if (analyser && isRecording) {
        const bufferLength = analyser.frequencyBinCount;
        const timeDomain = new Uint8Array(bufferLength);
        const freqData = new Uint8Array(bufferLength);
        analyser.getByteTimeDomainData(timeDomain);
        analyser.getByteFrequencyData(freqData);

        // Compute volume level from real microphone
        let sum = 0;
        for (let i = 0; i < freqData.length; i++) sum += freqData[i];
        const avg = sum / freqData.length / 255;
        setAudioLevel(avg);

        // Draw primary reactive neon waveform from physical mic time-domain data
        ctx.beginPath();
        ctx.strokeStyle = grad;
        ctx.lineWidth = 2.5;

        for (let i = 0; i < bufferLength; i++) {
          const x = (i / (bufferLength - 1)) * width;
          const v = timeDomain[i] / 128.0; // 0..2 (1.0 is center silence)
          const envelope = Math.sin((i / (bufferLength - 1)) * Math.PI);
          const y = centerY + (v - 1.0) * (height * 1.6) * envelope;

          if (i === 0) ctx.moveTo(x, y);
          else ctx.lineTo(x, y);
        }
        ctx.stroke();

        // Secondary subtle harmonic
        ctx.beginPath();
        ctx.strokeStyle = "rgba(231, 131, 154, 0.4)";
        ctx.lineWidth = 1.2;
        ctx.shadowColor = "#e7839a";
        for (let i = 0; i < bufferLength; i++) {
          const x = (i / (bufferLength - 1)) * width;
          const v = timeDomain[i] / 128.0;
          const envelope = Math.sin((i / (bufferLength - 1)) * Math.PI);
          const y = centerY + (v - 1.0) * (height * 0.9) * envelope * -1;
          if (i === 0) ctx.moveTo(x, y);
          else ctx.lineTo(x, y);
        }
        ctx.stroke();
      } else {
        // Resting ambient subtle neon baseline
        setAudioLevel(0);
        ctx.beginPath();
        ctx.strokeStyle = "rgba(0, 242, 254, 0.45)";
        ctx.lineWidth = 1.5;
        for (let i = 0; i < 60; i++) {
          const x = (i / 59) * width;
          const y = centerY + Math.sin(i * 0.2 + Date.now() * 0.003) * 2;
          if (i === 0) ctx.moveTo(x, y);
          else ctx.lineTo(x, y);
        }
        ctx.stroke();
      }

      animId = requestAnimationFrame(render);
    };

    animId = requestAnimationFrame(render);
    return () => cancelAnimationFrame(animId);
  }, [isRecording]);

  // Clean up audio resources on unmount
  useEffect(() => {
    return () => {
      if (streamRef.current) {
        streamRef.current.getTracks().forEach((t) => t.stop());
      }
      if (audioCtxRef.current) {
        audioCtxRef.current.close().catch(() => {});
      }
      if (speechRecognitionRef.current) {
        speechRecognitionRef.current.stop();
      }
    };
  }, []);

  // Start real microphone capture
  const startRecording = async () => {
    setMicError(null);
    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: {
          channelCount: 1,
          echoCancellation: true,
          noiseSuppression: true,
        },
      });

      streamRef.current = stream;
      const AudioCtxClass =
        window.AudioContext || (window as any).webkitAudioContext;
      const audioCtx = new AudioCtxClass();
      audioCtxRef.current = audioCtx;

      const source = audioCtx.createMediaStreamSource(stream);
      const analyser = audioCtx.createAnalyser();
      analyser.fftSize = 512;
      analyserRef.current = analyser;

      const scriptProcessor = audioCtx.createScriptProcessor(4096, 1, 1);
      scriptProcessorRef.current = scriptProcessor;
      audioChunksRef.current = [];
      isRecordingRef.current = true;

      scriptProcessor.onaudioprocess = (e) => {
        if (!isRecordingRef.current) return;
        const channelData = e.inputBuffer.getChannelData(0);
        audioChunksRef.current.push(new Float32Array(channelData));
      };

      source.connect(analyser);
      source.connect(scriptProcessor);
      scriptProcessor.connect(audioCtx.destination);

      setIsRecording(true);
      setMicStatusText("LISTENING… (Click to Finish)");
      setResponse("Listening to your voice… Speak now.");

      // Browser Web Speech API for instantaneous live partial captions
      if (
        typeof window !== "undefined" &&
        ("webkitSpeechRecognition" in window || "SpeechRecognition" in window)
      ) {
        try {
          const SpeechRec =
            (window as any).SpeechRecognition ||
            (window as any).webkitSpeechRecognition;
          const recognition = new SpeechRec();
          recognition.continuous = true;
          recognition.interimResults = true;
          recognition.onresult = (ev: any) => {
            let interim = "";
            for (let i = ev.resultIndex; i < ev.results.length; ++i) {
              interim += ev.results[i][0].transcript;
            }
            if (interim.trim()) {
              setQuery(interim);
            }
          };
          recognition.onerror = () => {};
          recognition.start();
          speechRecognitionRef.current = recognition;
        } catch {
          // SpeechRecognition optional fallback
        }
      }
    } catch (err: any) {
      console.error("Mic access failed", err);
      setMicError("Microphone permission denied or device not found.");
      setResponse(
        "Microphone access needed. Please grant mic permissions in your browser to test with your voice.",
      );
      setIsRecording(false);
      isRecordingRef.current = false;
    }
  };

  // Stop recording & transcribe voice through backend
  const stopRecording = async () => {
    isRecordingRef.current = false;
    setIsRecording(false);
    setMicStatusText("Processing Speech…");
    setIsProcessing(true);

    if (speechRecognitionRef.current) {
      try {
        speechRecognitionRef.current.stop();
      } catch {}
      speechRecognitionRef.current = null;
    }

    if (streamRef.current) {
      streamRef.current.getTracks().forEach((t) => t.stop());
      streamRef.current = null;
    }

    const inputSampleRate = audioCtxRef.current?.sampleRate || 44100;
    if (audioCtxRef.current) {
      audioCtxRef.current.close().catch(() => {});
      audioCtxRef.current = null;
    }

    // Merge audio chunks
    const chunks = audioChunksRef.current;
    let totalLength = 0;
    for (const c of chunks) totalLength += c.length;

    if (totalLength === 0) {
      setIsProcessing(false);
      setMicStatusText("Click to Activate");
      return;
    }

    const merged = new Float32Array(totalLength);
    let offset = 0;
    for (const c of chunks) {
      merged.set(c, offset);
      offset += c.length;
    }

    // Downsample to 16kHz & encode WAV
    const pcm16k = downsampleTo16k(merged, inputSampleRate);
    const wavBlob = encodeWAV(pcm16k, 16000);

    setResponse("Transcribing audio & scrubbing PII via EchoSync pipeline…");

    try {
      const startTime = performance.now();
      // Send real WAV audio to backend
      const res = await fetch(
        "/v1/audio?user_id=web_user&lat=-1.282&lng=36.821",
        {
          method: "POST",
          headers: { "Content-Type": "audio/wav" },
          body: wavBlob,
        },
      );

      if (!res.ok) {
        throw new Error(`Audio transcription returned ${res.status}`);
      }

      const audioData = await res.json();
      const elapsed = Math.round(performance.now() - startTime);
      setPipelineLatency(audioData.total_ms || elapsed);

      const transcribedText = (audioData.transcript || "").trim();
      const tokens = audioData.tokens || [];
      const pii = audioData.blocked_pii || [];

      setExtractedTokens(tokens);
      setBlockedPii(pii);

      if (transcribedText) {
        setQuery(transcribedText);
        setResponse("Generating AI reply…");

        // Ask the real LLM for a response
        const chatRes = await fetch("/api/chat", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ prompt: transcribedText }),
        });

        if (chatRes.ok) {
          const chatData = await chatRes.json();
          const reply = chatData.reply || "Processed voice command.";
          setResponse(reply);
          speakText(reply);
        } else {
          setResponse(`Transcribed: "${transcribedText}" (AI reply unavailable)`);
        }
      } else {
        setResponse("No clear speech detected. Please speak closer to your microphone and try again.");
      }
    } catch (err: any) {
      console.error("Transcription error:", err);
      // If server transcription failed but browser speech recognition got text
      if (query.trim()) {
        try {
          const chatRes = await fetch("/api/chat", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ prompt: query }),
          });
          if (chatRes.ok) {
            const chatData = await chatRes.json();
            const reply = chatData.reply;
            setResponse(reply);
            speakText(reply);
          }
        } catch {}
      } else {
        setResponse(`Speech error: ${err.message || "Failed to process audio"}`);
      }
    } finally {
      setIsProcessing(false);
      setMicStatusText("Click to Activate");
    }
  };

  // Toggle microphone recording
  const handleMicToggle = () => {
    if (isRecording) {
      stopRecording();
    } else {
      startRecording();
    }
  };

  // Handle manual question input
  const handleAsk = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!inputVal.trim()) return;

    const userPrompt = inputVal.trim();
    setQuery(userPrompt);
    setInputVal("");
    setIsProcessing(true);
    setResponse("EchoSync is processing…");

    try {
      // 1. Scrub PII and tokenize via backend
      const scrubRes = await fetch("/v1/scrub", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ user_id: "web_user", transcript: userPrompt }),
      });
      if (scrubRes.ok) {
        const scrubData = await scrubRes.json();
        setExtractedTokens(scrubData.tokens || []);
        setBlockedPii(scrubData.blocked_pii || []);
        setPipelineLatency(scrubData.scrub_latency_ms || 0);
      }

      // 2. Query real LLM
      const chatRes = await fetch("/api/chat", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt: userPrompt }),
      });
      if (chatRes.ok) {
        const chatData = await chatRes.json();
        const reply = chatData.reply || "EchoSync processed your query.";
        setResponse(reply);
        speakText(reply);
      } else {
        setResponse("Could not reach AI service. Please try again.");
      }
    } catch (err: any) {
      setResponse(`Error: ${err.message || "Failed to query AI"}`);
    } finally {
      setIsProcessing(false);
    }
  };

  // Generate real icebreaker from actual interests
  const handleFetchIcebreaker = async () => {
    setIsProcessing(true);
    setResponse("Requesting neural icebreaker from EchoSync backend…");
    try {
      const tokens =
        extractedTokens.length > 0
          ? extractedTokens
          : ["Realtime Voice", "Edge AI", "Privacy Computing"];

      const chatRes = await fetch("/api/chat", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          prompt: `Generate a fun, clever 1-2 sentence conversation starter or icebreaker for two people who both share an interest in: ${tokens.join(", ")}.`,
        }),
      });

      if (chatRes.ok) {
        const data = await chatRes.json();
        const reply = data.reply || "Ask them how they first got into tech.";
        setQuery(`Icebreaker for: ${tokens.join(", ")}`);
        setResponse(reply);
        speakText(reply);
      } else {
        setResponse("Could not generate icebreaker.");
      }
    } catch {
      setResponse("Could not generate icebreaker at this time.");
    } finally {
      setIsProcessing(false);
    }
  };

  return (
    <div
      ref={containerRef}
      className={`relative flex flex-col overflow-hidden text-paper transition-all duration-300 ${
        isFullscreen
          ? "fixed inset-0 z-50 h-screen w-screen rounded-none border-0 bg-[#050911] p-4 md:p-8"
          : "min-h-[640px] rounded-xl border border-cyan-500/20 bg-[#070b12] shadow-2xl"
      }`}
    >
      {/* Background radial bokeh & studio lighting */}
      <div className="pointer-events-none absolute inset-0">
        <div className="absolute -left-20 -top-20 h-96 w-96 rounded-full bg-cyan-500/10 blur-[120px]" />
        <div className="absolute -right-20 top-1/3 h-96 w-96 rounded-full bg-[#E7839A]/15 blur-[140px]" />
        <div className="absolute bottom-0 left-1/3 h-72 w-96 rounded-full bg-blue-600/10 blur-[130px]" />
        <div
          className="absolute inset-0 opacity-[0.03]"
          style={{
            backgroundImage: `radial-gradient(#00f2fe 1px, transparent 1px)`,
            backgroundSize: "24px 24px",
          }}
        />
      </div>

      {/* Top Header Controls / Switcher */}
      <div className="relative z-10 flex items-center justify-between border-b border-cyan-500/15 bg-[#090f1a]/80 px-6 py-3 backdrop-blur-md">
        {/* Brand */}
        <div className="flex items-center gap-3">
          <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-gradient-to-tr from-cyan-400 to-[#E7839A] shadow-[0_0_12px_rgba(0,242,254,0.5)]">
            <Sparkles size={15} className="text-white" />
          </div>
          <div>
            <h1 className="flex items-center gap-2 text-sm font-bold tracking-wider text-white">
              EchoSync
              <span className="rounded border border-cyan-400/30 bg-cyan-950/40 px-1.5 py-0.2 text-[9px] font-mono text-cyan-300">
                VOICE STUDIO
              </span>
            </h1>
          </div>
        </div>

        {/* Presentation View Toggle */}
        <div className="flex items-center gap-2 rounded-lg border border-cyan-500/20 bg-[#0d1624] p-1 text-xs">
          <button
            onClick={() => setViewMode("interactive")}
            className={`flex items-center gap-1.5 rounded px-3 py-1 text-[11px] font-medium transition-colors ${
              viewMode === "interactive"
                ? "bg-cyan-500/20 text-cyan-300 shadow-[0_0_8px_rgba(0,242,254,0.3)]"
                : "text-muted hover:text-paper"
            }`}
          >
            <Layers size={13} />
            Live Dashboard
          </button>
          <button
            onClick={() => setViewMode("monitor")}
            className={`flex items-center gap-1.5 rounded px-3 py-1 text-[11px] font-medium transition-colors ${
              viewMode === "monitor"
                ? "bg-cyan-500/20 text-cyan-300 shadow-[0_0_8px_rgba(0,242,254,0.3)]"
                : "text-muted hover:text-paper"
            }`}
          >
            <Monitor size={13} />
            Monitor Presentation
          </button>
        </div>

        {/* Telemetry Status Right & Fullscreen Button */}
        <div className="flex items-center gap-3 text-[11px] text-muted">
          <div className="hidden items-center gap-5 md:flex">
            <button
              onClick={() => setSpeechEnabled(!speechEnabled)}
              title={speechEnabled ? "Voice Speech Synthesis Enabled" : "Muted"}
              className="flex items-center gap-1 text-muted transition-colors hover:text-cyan-300"
            >
              {speechEnabled ? (
                <Volume2 size={13} className="text-cyan-400" />
              ) : (
                <VolumeX size={13} className="text-muted" />
              )}
              <span className="font-mono text-paper">
                {speechEnabled ? "TTS ON" : "MUTED"}
              </span>
            </button>
            <div className="flex items-center gap-1.5">
              <Wifi size={13} className="text-emerald-400" />
              <span className="text-[10px] text-emerald-300">
                {pipelineLatency ? `${pipelineLatency}ms` : "Active"}
              </span>
            </div>
            <div className="font-mono text-cyan-300">{currentTime}</div>
          </div>

          <button
            onClick={() =>
              isFullscreen
                ? exitFullscreen()
                : enterFullscreen(viewMode === "monitor")
            }
            className={`flex items-center gap-1.5 rounded-lg border px-3 py-1 text-xs font-medium transition-colors ${
              isFullscreen
                ? "border-pink-500/40 bg-pink-950/40 text-pink-300 shadow-[0_0_12px_rgba(231,131,154,0.3)] hover:bg-pink-900/50"
                : "border-cyan-500/20 bg-[#0d1624] text-muted hover:border-cyan-400/40 hover:text-cyan-300"
            }`}
            title={isFullscreen ? "Exit Fullscreen (Esc)" : "Full-Screen Mode"}
          >
            {isFullscreen ? <Minimize2 size={13} /> : <Maximize2 size={13} />}
            <span>{isFullscreen ? "Exit Fullscreen" : "Full Screen"}</span>
          </button>
        </div>
      </div>

      {/* Main View Area */}
      {viewMode === "monitor" ? (
        /* Sleek Desktop Monitor Presentation Mode */
        <div className="relative z-10 flex flex-1 flex-col items-center justify-center p-6 lg:p-10">
          <div
            onClick={() => enterFullscreen(true)}
            className="group relative w-full max-w-4xl cursor-pointer overflow-hidden rounded-2xl border border-cyan-500/30 bg-black/60 shadow-[0_20px_60px_rgba(0,0,0,0.8)] transition-all duration-300 hover:border-cyan-400/70 hover:shadow-[0_0_40px_rgba(0,242,254,0.3)]"
          >
            {/* The Presentation Image */}
            <img
              src="/assets/voice_ui_mockup.jpg"
              alt="Advanced Voice Application Interface on Sleek Desktop Monitor"
              className="h-auto w-full object-cover transition-transform duration-700 group-hover:scale-[1.01]"
            />

            {/* Hover overlay hint */}
            <div className="absolute inset-0 flex flex-col items-center justify-center bg-black/40 opacity-0 backdrop-blur-[2px] transition-all duration-300 group-hover:opacity-100">
              <div className="flex items-center gap-2 rounded-full border border-cyan-400/60 bg-[#070b12]/95 px-5 py-2.5 text-sm font-semibold text-cyan-300 shadow-[0_0_30px_rgba(0,242,254,0.6)] transition-transform duration-200 group-hover:scale-105">
                <Maximize2 size={16} className="animate-pulse text-cyan-400" />
                <span>Click to Open Full-Screen & Operate</span>
              </div>
              <p className="mt-2 text-xs font-mono text-cyan-300/80">
                Click anywhere to expand into live interactive mode
              </p>
            </div>

            {/* Overlay badge */}
            <div className="absolute bottom-4 left-4 right-4 flex items-center justify-between rounded-xl border border-white/10 bg-ink-950/80 p-3 backdrop-blur-md">
              <div>
                <p className="text-xs font-semibold text-white">
                  Studio UI Presentation · Unreal Engine 5 Render Style
                </p>
                <p className="text-[10px] text-muted">
                  Metallic fluid humanoid face · Holographic data ring · Neon audio waveform · Glassmorphic dialogue
                </p>
              </div>
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  enterFullscreen(true);
                }}
                className="flex items-center gap-1.5 rounded-lg border border-cyan-400/40 bg-cyan-500/20 px-3.5 py-1.5 text-xs font-medium text-cyan-300 shadow-[0_0_12px_rgba(0,242,254,0.3)] hover:bg-cyan-500/30"
              >
                <Maximize2 size={12} />
                <span>Launch Full-Screen →</span>
              </button>
            </div>
          </div>
        </div>
      ) : (
        /* Live Interactive Futuristic Dashboard UI */
        <div className="relative z-10 flex flex-1 flex-col justify-between p-6 lg:p-8">
          {/* Top HUD Metrics Bar */}
          <div className="flex items-center justify-between text-xs text-muted">
            <div className="flex items-center gap-3">
              <span className="flex items-center gap-1.5 rounded-full border border-emerald-500/30 bg-emerald-950/40 px-2.5 py-0.5 text-[10px] text-emerald-400">
                <span
                  className={`h-1.5 w-1.5 rounded-full ${
                    isRecording
                      ? "animate-ping bg-red-400"
                      : "animate-pulse bg-emerald-400"
                  }`}
                />
                {isRecording ? "MIC RECORDING LIVE" : "NEURAL CORE ONLINE"}
              </span>
              <span className="hidden font-mono text-[10px] text-muted sm:inline">
                {isRecording
                  ? "STREAMING PCM 16kHz · ANALYSER ACTIVE"
                  : "STANDBY · ZERO-TRUST PRIVACY ACTIVE"}
              </span>
            </div>
            <button
              onClick={handleFetchIcebreaker}
              disabled={isProcessing}
              className="flex items-center gap-1.5 rounded border border-cyan-500/30 bg-cyan-950/30 px-2.5 py-1 text-[10px] text-cyan-300 transition-colors hover:bg-cyan-500/20 disabled:opacity-50"
            >
              <RefreshCw
                size={11}
                className={isProcessing ? "animate-spin" : ""}
              />
              Generate Icebreaker
            </button>
          </div>

          {/* Centerpiece: Shimmering Fluid Face + Holographic Data Ring */}
          <div className="relative my-4 flex flex-1 flex-col items-center justify-center">
            {/* Ambient Backlight Glow */}
            <div className="pointer-events-none absolute h-72 w-72 rounded-full bg-gradient-to-br from-cyan-400/25 via-[#e7839a]/25 to-blue-500/20 blur-[60px]" />

            {/* Holographic Circular Data Ring Container */}
            <div className="relative flex items-center justify-center">
              {/* Outer Rotating Data Track Ring */}
              <div
                className="pointer-events-none absolute h-[290px] w-[290px] rounded-full border border-cyan-400/30 shadow-[0_0_20px_rgba(0,242,254,0.25)]"
                style={{
                  borderStyle: "dashed",
                  animation: "sweep-rotate 35s linear infinite",
                }}
              />

              {/* Counter-rotating Inner Telemetry Ring */}
              <div
                className="pointer-events-none absolute h-[264px] w-[264px] rounded-full border border-[#E7839A]/40"
                style={{
                  borderStyle: "dotted",
                  borderWidth: "1.5px",
                  animation: "sweep-rotate 22s linear infinite reverse",
                }}
              />

              {/* Glowing Holographic Cyan Circular Data Ring */}
              <div className="pointer-events-none absolute h-[242px] w-[242px] rounded-full border-2 border-cyan-400 shadow-[0_0_25px_rgba(0,242,254,0.6)]" />

              {/* Telemetry label around ring */}
              <div className="pointer-events-none absolute -top-4 font-mono text-[10px] font-bold tracking-widest text-cyan-300">
                EchoSync
              </div>

              {/* Shimmering Metallic Fluid Humanoid Face */}
              <div className="relative h-48 w-48 overflow-hidden rounded-full border-2 border-cyan-300/60 bg-black shadow-[0_0_35px_rgba(0,242,254,0.4)]">
                <img
                  src="/assets/ai_avatar.jpg"
                  alt="Metallic Fluid Humanoid Avatar"
                  className={`h-full w-full object-cover transition-transform duration-500 ${
                    isRecording || isSpeaking ? "scale-105" : "hover:scale-105"
                  }`}
                />
                {/* Iridescent ripple sheen overlay */}
                <div
                  className="pointer-events-none absolute inset-0 bg-gradient-to-t from-cyan-500/20 via-transparent to-pink-500/20 mix-blend-overlay"
                  style={{
                    animation: "blink-dot 4s ease-in-out infinite",
                  }}
                />
              </div>
            </div>

            {/* Left Side: Real Microphone Icon Button */}
            <div className="absolute left-2 top-1/2 -translate-y-1/2 lg:left-8">
              <div className="flex flex-col items-center gap-2">
                <button
                  onClick={handleMicToggle}
                  disabled={isProcessing}
                  className={`group relative flex h-16 w-16 items-center justify-center rounded-full border transition-all duration-300 ${
                    isRecording
                      ? "border-red-400 bg-red-950/70 shadow-[0_0_25px_rgba(248,113,113,0.6)]"
                      : isProcessing
                        ? "border-cyan-400 bg-cyan-950/60 shadow-[0_0_15px_rgba(0,242,254,0.4)]"
                        : "border-cyan-400/50 bg-ink-900/70 text-cyan-300 hover:border-cyan-300 hover:shadow-[0_0_20px_rgba(0,242,254,0.4)]"
                  }`}
                  title={
                    isRecording
                      ? "Click to Stop & Transcribe"
                      : "Click to Speak"
                  }
                >
                  {/* Real-time pulsing aura when active */}
                  {isRecording && (
                    <span className="absolute inset-0 animate-ping rounded-full bg-red-400/30" />
                  )}
                  {isProcessing ? (
                    <Loader2 size={24} className="animate-spin text-cyan-300" />
                  ) : isRecording ? (
                    <Mic size={24} className="text-red-400" />
                  ) : (
                    <Mic size={24} className="text-cyan-300" />
                  )}
                </button>
                <span className="text-[10px] font-medium tracking-wide text-cyan-300/90 text-center max-w-[120px]">
                  {isProcessing
                    ? "PROCESSING…"
                    : isRecording
                      ? "RECORDING (Click to Finish)"
                      : micStatusText}
                </span>

                {/* Vertical real-time audio meter bars */}
                <div className="flex h-6 items-end gap-1">
                  {[0.3, 0.7, 1.0, 0.8, 0.5].map((bar, i) => (
                    <span
                      key={i}
                      className="w-1.5 rounded-full bg-cyan-400 transition-all duration-75"
                      style={{
                        height: isRecording
                          ? `${Math.max(15, bar * audioLevel * 100)}%`
                          : "15%",
                        opacity: isRecording ? 0.95 : 0.25,
                      }}
                    />
                  ))}
                </div>
              </div>
            </div>

            {/* Right Side: Futuristic Audio Telemetry HUD */}
            <div className="absolute right-2 top-1/2 hidden -translate-y-1/2 font-mono text-[10px] text-muted md:block lg:right-8">
              <div className="flex flex-col gap-2 rounded-lg border border-cyan-500/15 bg-[#090f1a]/70 p-3.5 backdrop-blur-sm shadow-lg max-w-[200px]">
                <div className="flex items-center gap-1.5 text-[9px] font-bold text-cyan-300">
                  <ShieldCheck size={12} />
                  <span>PRIVACY TELEMETRY</span>
                </div>
                <div className="flex justify-between gap-3">
                  <span>INPUT:</span>
                  <span className="text-paper">
                    {isRecording ? "16kHz Mono" : "Ready"}
                  </span>
                </div>
                <div className="flex justify-between gap-3">
                  <span>FIREWALL:</span>
                  <span className="text-emerald-400">Groq Zero-Trust</span>
                </div>
                <div className="flex justify-between gap-3">
                  <span>LATENCY:</span>
                  <span className="text-paper">
                    {pipelineLatency ? `${pipelineLatency}ms` : "—"}
                  </span>
                </div>
                <div className="flex justify-between gap-3">
                  <span>TOKENS:</span>
                  <span className="text-cyan-300">
                    {extractedTokens.length > 0
                      ? `${extractedTokens.length} extracted`
                      : "0"}
                  </span>
                </div>
                {blockedPii && blockedPii.length > 0 && (
                  <div className="flex justify-between gap-3 border-t border-line/40 pt-1 text-[9px] text-amber-300">
                    <span>BLOCKED PII:</span>
                    <span>{blockedPii.length} redacted</span>
                  </div>
                )}
              </div>
            </div>

            {/* Below Face: Sharp Clean Neon Audio Waveform */}
            <div className="mt-5 flex w-full max-w-lg flex-col items-center">
              <canvas
                ref={canvasRef}
                width={520}
                height={56}
                className="w-full max-w-md"
              />
            </div>
          </div>

          {/* Bottom: Clean Translucent Glassmorphic Text Box */}
          <div className="relative z-20 mx-auto w-full max-w-2xl">
            <div className="rounded-2xl border border-cyan-500/25 bg-[#0d1626]/80 p-4 shadow-[0_8px_32px_rgba(0,0,0,0.5)] backdrop-blur-xl">
              {/* Mic error notice */}
              {micError && (
                <div className="mb-2 rounded border border-amber-500/30 bg-amber-950/30 p-2 text-xs text-amber-300">
                  {micError}
                </div>
              )}

              {/* User Query / Voice Transcript */}
              <div className="flex items-start gap-2.5 text-xs text-paper">
                <span className="font-semibold text-muted">User:</span>
                <p className="flex-1 font-medium text-slate-200">
                  {query || (
                    <span className="italic text-muted/60">
                      Click the mic button to speak with EchoSync…
                    </span>
                  )}
                </p>
              </div>

              {/* Extracted Tokens Badge */}
              {extractedTokens.length > 0 && (
                <div className="mt-2 flex flex-wrap items-center gap-1.5 border-t border-cyan-500/10 pt-2">
                  <span className="text-[10px] font-mono text-cyan-400/80">
                    Privacy Tokens:
                  </span>
                  {extractedTokens.map((tok, idx) => (
                    <span
                      key={idx}
                      className="rounded border border-cyan-400/30 bg-cyan-950/40 px-1.5 py-0.2 text-[9px] font-mono text-cyan-300"
                    >
                      {tok}
                    </span>
                  ))}
                </div>
              )}

              {/* AI Response */}
              <div className="mt-2.5 flex items-start gap-2.5 border-t border-cyan-500/15 pt-2.5 text-xs">
                <span className="font-semibold text-cyan-400">EchoSync:</span>
                <p className="flex-1 leading-relaxed text-slate-300">
                  {response}
                </p>
              </div>

              {/* Interactive Query Input */}
              <form
                onSubmit={handleAsk}
                className="mt-3 flex items-center gap-2 border-t border-line/30 pt-2"
              >
                <input
                  type="text"
                  value={inputVal}
                  onChange={(e) => setInputVal(e.target.value)}
                  placeholder="Ask EchoSync a question or type a query…"
                  disabled={isProcessing}
                  className="flex-1 rounded-lg border border-cyan-500/20 bg-black/40 px-3 py-1.5 text-xs text-paper placeholder:text-muted/60 focus:border-cyan-400 focus:outline-none disabled:opacity-50"
                />
                <button
                  type="submit"
                  disabled={isProcessing || !inputVal.trim()}
                  className="flex items-center gap-1 rounded-lg bg-cyan-500/20 px-3 py-1.5 text-xs font-medium text-cyan-300 transition-colors hover:bg-cyan-500/30 disabled:opacity-40"
                >
                  <Send size={12} />
                  <span>Send</span>
                </button>
              </form>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
