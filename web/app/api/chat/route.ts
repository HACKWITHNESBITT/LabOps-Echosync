import { NextResponse } from "next/server";

export async function POST(req: Request) {
  try {
    const body = await req.json();
    const prompt = body.prompt || "";
    const history = body.history || [];

    if (!prompt.trim()) {
      return NextResponse.json({ error: "Empty prompt" }, { status: 400 });
    }

    const apiKey =
      process.env.GROQ_API_KEY ||
      "process.env.GROQ_API_KEY";
    const model = process.env.GROQ_MODEL || "openai/gpt-oss-120b";
    const baseURL =
      process.env.GROQ_BASE_URL || "https://api.groq.com/openai/v1";

    const response = await fetch(`${baseURL}/chat/completions`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${apiKey}`,
      },
      body: JSON.stringify({
        model,
        messages: [
          {
            role: "system",
            content:
              "You are EchoSync, a privacy-first ambient intelligence AI assistant. Answer user queries concisely, accurately, and naturally in 1 to 3 spoken sentences suitable for voice synthesis.",
          },
          ...history.slice(-4),
          { role: "user", content: prompt },
        ],
        temperature: 0.7,
        max_tokens: 220,
      }),
    });

    if (!response.ok) {
      const errText = await response.text();
      return NextResponse.json(
        { error: `LLM service returned ${response.status}: ${errText}` },
        { status: response.status }
      );
    }

    const data = await response.json();
    const reply =
      data.choices?.[0]?.message?.content ||
      "EchoSync processed your request, but no response was generated.";

    return NextResponse.json({ reply });
  } catch (err: any) {
    return NextResponse.json(
      { error: err.message || "Failed to process chat" },
      { status: 500 }
    );
  }
}
