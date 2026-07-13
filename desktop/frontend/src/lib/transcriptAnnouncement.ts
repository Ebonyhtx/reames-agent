export type TranscriptAnnouncementState = {
  sessionKey: string;
  assistantId: string;
  wasRunning: boolean;
  wasSuppressed: boolean;
};

export type CompletedAssistant = {
  id: string;
  text: string;
};

export type TranscriptAnnouncementResolution = {
  state: TranscriptAnnouncementState;
  announcement?: CompletedAssistant;
  sessionChanged: boolean;
};

export const EMPTY_TRANSCRIPT_ANNOUNCEMENT_STATE: TranscriptAnnouncementState = {
  sessionKey: "",
  assistantId: "",
  wasRunning: false,
  wasSuppressed: false,
};

export function resolveTranscriptAnnouncement(
  previous: TranscriptAnnouncementState,
  sessionKey: string,
  running: boolean,
  suppressed: boolean,
  latest?: CompletedAssistant,
): TranscriptAnnouncementResolution {
  if (previous.sessionKey !== sessionKey) {
    return {
      state: { sessionKey, assistantId: latest?.id ?? "", wasRunning: running, wasSuppressed: suppressed },
      sessionChanged: true,
    };
  }
  if (suppressed || previous.wasSuppressed) {
    return {
      state: { sessionKey, assistantId: latest?.id ?? previous.assistantId, wasRunning: running, wasSuppressed: suppressed },
      sessionChanged: false,
    };
  }
  if (running) {
    return { state: { ...previous, wasRunning: true }, sessionChanged: false };
  }
  if (!latest || previous.assistantId === latest.id || !previous.wasRunning) {
    return {
      state: { ...previous, assistantId: latest?.id ?? previous.assistantId, wasRunning: false },
      sessionChanged: false,
    };
  }
  return {
    state: { sessionKey, assistantId: latest.id, wasRunning: false, wasSuppressed: false },
    announcement: latest,
    sessionChanged: false,
  };
}
