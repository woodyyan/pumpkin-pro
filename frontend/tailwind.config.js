/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./pages/**/*.{js,ts,jsx,tsx}",
    "./components/**/*.{js,ts,jsx,tsx}",
    "./lib/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        background: "#0a0a0b",
        foreground: "#ededed",
        card: "#161618",
        border: "rgba(255,255,255,0.1)",
        primary: "#e67e22", // Pumpkin orange
        positive: "#22c55e",
        negative: "#ef4444"
      },
    },
  },
  plugins: [],
}