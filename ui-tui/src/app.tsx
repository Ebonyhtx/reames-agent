import { useStore } from '@nanostores/react'
import { useCallback, useState } from 'react'

import { GatewayProvider } from './app/gatewayContext.js'
import { $uiState } from './app/uiStore.js'
import { useMainApp } from './app/useMainApp.js'
import { AppLayout } from './components/appLayout.js'
import type { GatewayClient } from './gatewayClient.js'
import { HomeScreen } from './routes/homeScreen.js'

export function App({ gw }: { gw: GatewayClient }) {
  const { appActions, appComposer, appProgress, appStatus, appTranscript, gateway } = useMainApp(gw)
  const { mouseTracking } = useStore($uiState)
  const [screen, setScreen] = useState<'home' | 'session'>('home')

  const handleHomeSubmit = useCallback((query: string) => {
    setScreen('session')
    // Small delay to let the session layout mount before submitting
    setTimeout(() => {
      appActions.newPromptSession(query)
    }, 50)
  }, [appActions])

  return (
    <GatewayProvider value={gateway}>
      {screen === 'home' ? (
        <HomeScreen onStart={handleHomeSubmit} />
      ) : (
        <AppLayout
          actions={appActions}
          composer={appComposer}
          mouseTracking={mouseTracking}
          progress={appProgress}
          status={appStatus}
          transcript={appTranscript}
        />
      )}
    </GatewayProvider>
  )
}
