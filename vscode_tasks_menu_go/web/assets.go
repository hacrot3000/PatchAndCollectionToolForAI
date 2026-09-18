package webassets

import "embed"

// Files contains the browser-side terminal dependencies and progressive UI
// enhancements required by the web UI. They are embedded so the final Go
// executable does not require Internet access at runtime.
//
//go:embed features.js featuremods/*.js vendor/xterm.js vendor/xterm.css vendor/addon-fit.js vendor/LICENSE-xterm.txt vendor/LICENSE-addon-fit.txt vendor/VERSION.txt vendor/codemirror6-all.min.js vendor/LICENSE-codemirror6-prebuilt.txt vendor/VERSION-codemirror6.txt
var Files embed.FS
