import type { Config } from "tailwindcss";

const preset: Partial<Config> = {
  theme: {
    extend: {
      colors: {
        primary: {
          DEFAULT: "#FF2D8A",
          50: "#FFF0F6",
          100: "#FFE0ED",
          200: "#FFC2DB",
          300: "#FF94BF",
          400: "#FF5DA0",
          500: "#FF2D8A",
          600: "#E0186F",
          700: "#BD1060",
          800: "#9B1054",
          900: "#81124A",
        },
        surface: {
          DEFAULT: "#F8F9FA",
          50: "#FFFFFF",
          100: "#F8F9FA",
          200: "#E9ECEF",
          300: "#DEE2E6",
          400: "#CED4DA",
        },
        status: {
          pending: "#F59E0B",
          approved: "#10B981",
          rejected: "#EF4444",
          "in-progress": "#3B82F6",
          submitted: "#8B5CF6",
          completed: "#6B7280",
        },
      },
      fontFamily: {
        sans: [
          "Inter",
          "system-ui",
          "-apple-system",
          "Segoe UI",
          "Roboto",
          "sans-serif",
        ],
      },
      borderRadius: {
        card: "12px",
        button: "8px",
      },
    },
  },
};

export default preset;
