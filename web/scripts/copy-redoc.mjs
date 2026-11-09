import { copyFileSync, mkdirSync } from 'node:fs'

mkdirSync('public/vendor', { recursive: true })
copyFileSync('node_modules/redoc/bundles/redoc.standalone.js', 'public/vendor/redoc.standalone.js')
