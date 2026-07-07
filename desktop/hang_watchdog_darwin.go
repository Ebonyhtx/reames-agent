//go:build darwin

package main

/*
#include <stdint.h>
#include <dispatch/dispatch.h>

extern void reamesAgentDesktopMainHeartbeat(void);

static dispatch_source_t reames_agent_main_heartbeat_timer;

static void reames_agent_main_heartbeat_handler(void *ctx) {
	reamesAgentDesktopMainHeartbeat();
}

static void reames_agent_start_main_heartbeat(uint64_t interval_ms) {
	if (reames_agent_main_heartbeat_timer != NULL) {
		return;
	}
	reames_agent_main_heartbeat_timer = dispatch_source_create(DISPATCH_SOURCE_TYPE_TIMER, 0, 0, dispatch_get_main_queue());
	dispatch_set_context(reames_agent_main_heartbeat_timer, NULL);
	dispatch_source_set_event_handler_f(reames_agent_main_heartbeat_timer, reames_agent_main_heartbeat_handler);
	dispatch_source_set_timer(reames_agent_main_heartbeat_timer, dispatch_time(DISPATCH_TIME_NOW, 0), interval_ms * NSEC_PER_MSEC, 100 * NSEC_PER_MSEC);
	dispatch_resume(reames_agent_main_heartbeat_timer);
}

static void reames_agent_stop_main_heartbeat(void) {
	if (reames_agent_main_heartbeat_timer == NULL) {
		return;
	}
	dispatch_source_cancel(reames_agent_main_heartbeat_timer);
	reames_agent_main_heartbeat_timer = NULL;
}
*/
import "C"

import "time"

func mainThreadWatchdogSupported() bool {
	return true
}

func startNativeMainThreadHeartbeat(intervalMS uint64) {
	C.reames_agent_start_main_heartbeat(C.uint64_t(intervalMS))
}

func stopNativeMainThreadHeartbeat() {
	C.reames_agent_stop_main_heartbeat()
}

//export reamesAgentDesktopMainHeartbeat
func reamesAgentDesktopMainHeartbeat() {
	recordMainThreadHeartbeat(time.Now())
}
