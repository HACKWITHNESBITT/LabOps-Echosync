import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./app/**/*.{ts,tsx}", "./components/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        ink: {
          950: "#0B1220",
          900: "#111A29",
          850: "#17243A",
          800: "#202D40",
        },
        line: "#657187",
        pulse: "#E7839A",
        muted: "#7C8797",
        paper: "#E1E6ED",
      },
      fontFamily: {
        mono: ["ui-monospace", "SFMono-Regular", "Menlo", "monospace"],
      },
    },
  },
  plugins: [],
};
export default config;