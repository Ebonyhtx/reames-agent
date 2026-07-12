package boot

// Compile-time provider implementations register themselves with the shared
// provider registry during boot package initialization. Frontends import boot
// for assembly and no longer need their own blank-import composition roots.
import (
	_ "reames-agent/internal/provider/anthropic"
	_ "reames-agent/internal/provider/openai"
)
