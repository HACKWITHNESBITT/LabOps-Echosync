import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "EchoSync · Command Center",
  description:
    "Live PII scrubbing pipeline, proximity match radar and ephemeral match feed.",
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body className="bg-ink-950 text-paper antialiased">{children}</body>
    </html>
  );
}