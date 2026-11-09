import { init } from '/vendor/redoc.standalone.js'

init(
  '/api/openapi.yaml',
  {
    disableTelemetry: true,
    hideDownloadButton: false,
    theme: {
      colors: { primary: { main: '#1678D3' } },
      rightPanel: { backgroundColor: '#092653' },
      typography: { fontFamily: 'system-ui, sans-serif', headings: { fontFamily: 'system-ui, sans-serif' } },
    },
  },
  document.getElementById('redoc'),
)
