import type { Config } from "tailwindcss";

// The palette is intentionally not the default AI look: no purple→blue gradient,
// no stock indigo. It's a warm-brass-on-midnight-teal "amphitheater at night".
const config: Config = {
  content: ["./app/**/*.{ts,tsx}", "./components/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        night: {
          900: "#080d0f",
          800: "#0c1417",
          700: "#111e22",
          600: "#16292e",
        },
        brass: {
          300: "#f3d18a",
          400: "#e6b45c",
          500: "#d19a3f",
          600: "#a9772a",
        },
        steel: {
          300: "#9db6c2",
          400: "#6f909e",
          500: "#4d6b78",
        },
        seat: {
          available: "#6f909e",
          held: "#e6a23a",
          mine: "#ffd27a",
          booked: "#2f434b",
        },
      },
      fontFamily: {
        display: ["var(--font-display)", "Georgia", "serif"],
        sans: ["var(--font-sans)", "system-ui", "sans-serif"],
      },
      keyframes: {
        "pulse-hold": {
          "0%, 100%": { opacity: "1" },
          "50%": { opacity: "0.55" },
        },
        "glow-mine": {
          "0%, 100%": { filter: "drop-shadow(0 0 3px rgba(255,210,122,0.7))" },
          "50%": { filter: "drop-shadow(0 0 9px rgba(255,210,122,1))" },
        },
        "rise": {
          "0%": { opacity: "0", transform: "translateY(8px)" },
          "100%": { opacity: "1", transform: "translateY(0)" },
        },
      },
      animation: {
        "pulse-hold": "pulse-hold 1.6s ease-in-out infinite",
        "glow-mine": "glow-mine 1.4s ease-in-out infinite",
        "rise": "rise 0.28s ease-out",
      },
    },
  },
  plugins: [],
};

export default config;
