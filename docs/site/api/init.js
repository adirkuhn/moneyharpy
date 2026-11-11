// The published API reference: the same Redoc view as /docs/api/ in the
// app, reading the contract published next to it.
import { init } from './redoc.standalone.js'

init(
  'openapi.yaml',
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
