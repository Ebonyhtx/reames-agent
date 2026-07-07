import { Box, Text } from '@hermes/ink'
import { useStore } from '@nanostores/react'
import { useMemo, useState } from 'react'

import { $uiState } from '../app/uiStore.js'
import { AnimatedLogo } from '../components/logoAnimation.js'
import { TextInput } from '../components/textInput.js'
import { PLACEHOLDERS } from '../content/placeholders.js'

export function HomeScreen({ onStart }: { onStart: (query: string) => void }) {
  const ui = useStore($uiState)
  const [input, setInput] = useState('')

  const placeholder = useMemo(() => {
    return PLACEHOLDERS[Math.floor(Math.random() * PLACEHOLDERS.length)] ?? 'Type a message…'
  }, [])

  const handleSubmit = () => {
    const trimmed = input.trim()

    if (trimmed) {
      onStart(trimmed)
      setInput('')
    }
  }

  return (
    <Box flexDirection="column" alignItems="center" justifyContent="center" width="100%" paddingTop={3}>
      {/* Spacer to push logo toward center */}
      <Box flexGrow={1} minHeight={1} />

      {/* Animated REAMES Logo */}
      <AnimatedLogo t={ui.theme} />

      <Box height={2} />

      {/* Tagline */}
      <Text color={ui.theme.color.muted} dimColor>
        Where Models and Agents Co-Evolve
      </Text>

      <Box height={3} />

      {/* Input area */}
      <Box
        borderColor={ui.theme.color.border}
        borderStyle="round"
        paddingX={2}
        paddingY={1}
        width={60}
      >
        <Box flexGrow={1}>
          <Text color={ui.theme.color.prompt}>❯ </Text>
          <TextInput
            value={input}
            onChange={setInput}
            onSubmit={handleSubmit}
            placeholder={placeholder}
          />
        </Box>
      </Box>

      {/* Help hint */}
      <Box height={2} />

      <Text color={ui.theme.color.muted} dimColor wrap="truncate-end">
        Type a message and press Enter to start chatting
      </Text>

      {/* Push bottom */}
      <Box flexGrow={1} minHeight={1} />
    </Box>
  )
}
