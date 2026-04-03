import { useState } from 'react'
import { LoginModal } from './LoginModal'
import { SignupModal } from './SignupModal'

export function AuthGate() {
    const [mode, setMode] = useState<'login' | 'signup'>('login')

    if (mode === 'signup') {
        return <SignupModal onSwitchToLogin={() => setMode('login')} />
    }

    return <LoginModal onSwitchToSignup={() => setMode('signup')} />
}
