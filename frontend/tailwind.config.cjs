/** @type {import('tailwindcss').Config} */
module.exports = {
  darkMode: "class",
  content: [
    "./pages/**/*.{js,ts,jsx,tsx}",
    "./components/**/*.{js,ts,jsx,tsx}",
    "./lib/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        // semantic tokens driven by CSS variables — supports light/dark themes
        background:       "var(--color-bg-primary)",
        "background-alt": "var(--color-bg-secondary)",
        card:             "var(--color-bg-card)",
        foreground:       "var(--color-text-primary)",
        "foreground-muted":"var(--color-text-secondary)",
        "foreground-dim": "var(--color-text-tertiary)",
        "foreground-disabled":"var(--color-text-disabled)",
        border:           "var(--color-border)",
        "border-strong":  "var(--color-border-strong)",
        primary:          "var(--color-primary)",
        positive:         "var(--color-positive)",
        negative:         "var(--color-negative)",
      },
    },
  },
  plugins: [],
}