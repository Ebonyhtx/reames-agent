//go:build linux

package main

/*
#cgo linux pkg-config: gtk+-3.0
#cgo !webkit2_41 pkg-config: webkit2gtk-4.0
#cgo webkit2_41 pkg-config: webkit2gtk-4.1

#include <errno.h>
#include <signal.h>
#include <stdio.h>
#include <string.h>

#include <glib.h>

static void reames_agent_fix_signal(int signum)
{
	struct sigaction st;

	if (sigaction(signum, NULL, &st) < 0) {
		goto fix_signal_error;
	}
	st.sa_flags |= SA_ONSTACK;
	if (sigaction(signum, &st, NULL) < 0) {
		goto fix_signal_error;
	}
	return;

fix_signal_error:
	fprintf(stderr, "reames-agent: error fixing handler for signal %d: %s\n",
		signum, strerror(errno));
}

static void reames_agent_install_signal_handlers(void)
{
#if defined(SIGCHLD)
	reames_agent_fix_signal(SIGCHLD);
#endif
#if defined(SIGHUP)
	reames_agent_fix_signal(SIGHUP);
#endif
#if defined(SIGINT)
	reames_agent_fix_signal(SIGINT);
#endif
#if defined(SIGQUIT)
	reames_agent_fix_signal(SIGQUIT);
#endif
#if defined(SIGABRT)
	reames_agent_fix_signal(SIGABRT);
#endif
#if defined(SIGFPE)
	reames_agent_fix_signal(SIGFPE);
#endif
#if defined(SIGTERM)
	reames_agent_fix_signal(SIGTERM);
#endif
#if defined(SIGBUS)
	reames_agent_fix_signal(SIGBUS);
#endif
#if defined(SIGSEGV)
	reames_agent_fix_signal(SIGSEGV);
#endif
	// JavaScriptCore owns SIGUSR1 for conservative GC stack scanning.
#if defined(SIGXCPU)
	reames_agent_fix_signal(SIGXCPU);
#endif
#if defined(SIGXFSZ)
	reames_agent_fix_signal(SIGXFSZ);
#endif
}

static gboolean reames_agent_install_signal_handlers_timeout(gpointer data)
{
	reames_agent_install_signal_handlers();
	int *remaining = (int *)data;
	(*remaining)--;
	return *remaining > 0 ? G_SOURCE_CONTINUE : G_SOURCE_REMOVE;
}

static void reames_agent_schedule_signal_handler_fix(void)
{
	int *remaining = (int *)g_malloc(sizeof(int));
	*remaining = 100;
	g_timeout_add_full(
		G_PRIORITY_DEFAULT,
		50,
		reames_agent_install_signal_handlers_timeout,
		remaining,
		g_free
	);
}
*/
import "C"

// configureWebKitRendererRecovery applies WebKit's broad DMA-BUF fallback only
// during Safe Mode on NVIDIA systems. Normal launches keep acceleration; the
// narrower NVIDIA Wayland explicit-sync workaround remains independently active.
func configureWebKitRendererRecovery(safeMode bool) {
	if !safeMode {
		return
	}
	configureWebKitRendererRecoveryForGPU(safeMode, hasNVIDIAGPU())
}

// scheduleWebKitSignalHandlerRepair covers JavaScriptCore's lazy signal-handler
// installation window after Wails enters GTK's main loop. It mirrors the final
// Wails v2.13 Linux repair while Reames remains on the verified Wails v2.12 line.
func scheduleWebKitSignalHandlerRepair() {
	C.reames_agent_schedule_signal_handler_fix()
}

// repairWebKitSignalHandlers performs one deterministic repair after the DOM is
// ready, when JavaScriptCore has installed its lazy handlers.
func repairWebKitSignalHandlers() {
	C.reames_agent_install_signal_handlers()
}
