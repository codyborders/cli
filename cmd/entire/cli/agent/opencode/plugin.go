package opencode

import _ "embed"

//go:embed entire_plugin.ts
var pluginTemplate string

// entireCmdPlaceholder is replaced with the actual command during installation.
const entireCmdPlaceholder = "__ENTIRE_CMD__"

// entireWarningPlaceholder is replaced with the warning shown when `entire`
// is not available on PATH for production session-start hooks.
const entireWarningPlaceholder = "__ENTIRE_NOT_INSTALLED_WARNING__"
