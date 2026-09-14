"use client";

import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { VoiceInterface } from "@/components/voice-interface";

export default function VoicePage() {
  return (
    <div className="flex min-h-screen flex-col bg-[#050911] text-paper">
      <header className="flex items-center justify-between border-b border-cyan-500/20 bg-[#080d17] px-6 py-3">
        <div className="flex items-center gap-4">
          <Link
            href="/"
            className="flex items-center gap-1.5 rounded-lg border border-cyan-500/20 bg-cyan-950/30 px-3 py-1 text-xs text-cyan-300 transition-colors hover:bg-cyan-500/20"
          >
            <ArrowLeft size={13} />
            Command Center
          </Link>
          <div className="border-l border-line/40 pl-4">
            <h1 className="text-sm font-bold tracking-wide text-white">EchoSync · Voice Studio</h1>
            <p className="text-[10px] text-muted">Advanced Voice Application Interface</p>
          </div>
        </div>
      </header>

      <main className="flex flex-1 flex-col p-4 md:p-6">
        <div className="flex-1">
          <VoiceInterface />
        </div>
      </main>
    </div>
  );
}
