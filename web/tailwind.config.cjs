module.exports = {
  content: ['./index.html', './src/**/*.{svelte,ts}'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['Inter', 'ui-sans-serif', 'system-ui', 'sans-serif']
      }
    }
  },
  daisyui: {
    themes: [
      {
        'novelforge-light': {
          primary: '#6d28d9',
          secondary: '#0f766e',
          accent: '#b45309',
          neutral: '#1f2937',
          'base-100': '#ffffff',
          'base-200': '#f4f4f5',
          'base-300': '#e4e4e7',
          info: '#0369a1',
          success: '#15803d',
          warning: '#a16207',
          error: '#b91c1c'
        }
      },
      {
        'novelforge-dark': {
          primary: '#a78bfa',
          secondary: '#5eead4',
          accent: '#fbbf24',
          neutral: '#111827',
          'base-100': '#111318',
          'base-200': '#191c23',
          'base-300': '#252a34',
          info: '#38bdf8',
          success: '#4ade80',
          warning: '#facc15',
          error: '#f87171'
        }
      }
    ]
  },
  plugins: [require('daisyui')]
};
