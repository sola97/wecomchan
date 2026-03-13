/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{js,jsx}"],
  theme: {
    extend: {
      colors: {
        clay: "#a95a17",
        ink: "#0f1418",
        graphite: "#11161c",
        panel: "#1b232c",
        panelSoft: "#22303d",
        paper: "#f5efe3",
        sand: "#cabfae",
        ember: "#ef6b47",
        cyan: "#7ee7e1",
        mint: "#7fd1a0",
      },
      fontFamily: {
        sans: ["IBM Plex Sans", "PingFang SC", "Hiragino Sans GB", "sans-serif"],
        mono: ["IBM Plex Mono", "ui-monospace", "SFMono-Regular", "monospace"],
      },
      boxShadow: {
        panel: "0 24px 70px rgba(148, 163, 184, 0.18)",
        haze: "0 24px 80px rgba(0, 0, 0, 0.35)",
      },
      backgroundImage: {
        grain:
          "radial-gradient(circle at 15% 20%, rgba(126, 231, 225, 0.12), transparent 32%), radial-gradient(circle at 85% 15%, rgba(239, 107, 71, 0.18), transparent 28%), linear-gradient(135deg, rgba(255,255,255,0.03), rgba(255,255,255,0))",
      },
    },
  },
  plugins: [],
};
