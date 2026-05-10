// Tailwind v4 uses its own PostCSS plugin (@tailwindcss/postcss).
// autoprefixer is built into Tailwind v4 — no longer a separate step.
export default {
  plugins: {
    '@tailwindcss/postcss': {},
  },
}
